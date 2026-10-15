package readers

import (
	"bufio"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/gputop/gputop-go/internal/model"
)

// Per-block utilisation, by supervising a radeontop subprocess.
//
// Why a subprocess at all: the amdgpu driver exposes its GRBM and SRBM busy
// counters only through the radeon ioctl, on a handle that needs elevated
// privileges. gputop's whole design is that it never requires root, so it
// cannot take that dependency for the whole application -- but the counters
// are the only way to see *which* part of the graphics pipe is saturated,
// which is what "why is my GPU at 100%" actually reduces to. They are read the
// one way that does not make gputop itself privileged: by running radeontop as
// a child process and parsing its dump output. The privilege requirement is
// therefore exactly whatever radeontop needs, which stays a per-machine choice
// rather than becoming this program's.
//
// Degradation: a missing binary, a kernel that refuses the ioctl, an
// unsupported card and a crash are four different problems with four different
// remedies, so they are four different BlocksStatus values rather than one
// "unavailable". None of them stop the sampler, and the panel is hidden rather
// than showing empty bars.
//
// This is the only file in the package allowed to launch a process (enforced
// by TestReadersLaunchOnlyRadeontop): a shell can redirect to a path no Go
// call here ever names, so the launcher is confined to the one module whose
// whole purpose is the supervised child.

// radeontopHeader matches the dump line's prefix: "<seconds>.<microseconds>: bus 0c, ...".
var radeontopHeader = regexp.MustCompile(`^\s*(\d+)\.(\d+)\s*:\s*bus\s+([0-9a-fA-F]+)\s*,?\s*(.*)\n?$`)

// radeontopField matches one "key value..." field of the dump line.
var radeontopField = regexp.MustCompile(`^([a-z][a-z0-9_]*)\s+(.*)$`)

// radeontopNumber matches a number inside a field's value list.
var radeontopNumber = regexp.MustCompile(`-?\d+(?:\.\d+)?`)

// MinDumpIntervalS is radeontop's own dump interval granularity. -i is parsed
// with atoi and clamped to a minimum of 1 second, so a sub-second interval is
// not expressible and pretending otherwise would mean silently sampling slower
// than the user asked for.
const MinDumpIntervalS = 1

// MaxDumpIntervalS bounds the requested dump interval, matching the reference.
const MaxDumpIntervalS = 3600

// DefaultTicks is the default samples per second requested from radeontop.
const DefaultTicks = 120

// DefaultRadeontopBinary is the program name looked up on $PATH.
const DefaultRadeontopBinary = "radeontop"

// DefaultDrmNodeRoot is where the DRM nodes live, so -p can be preferred to -b.
const DefaultDrmNodeRoot = "/dev/dri"

// StartupGraceS is how long to wait for the first line before deciding the
// child is not going to produce one. radeontop busy-waits for its initial
// sample set before its first dump, so a short grace period is not enough.
const StartupGraceS = 3.0

// Retained diagnostic windows. radeontop is chatty on failure and nearly
// silent on success, so small windows are plenty.
const (
	stderrWindow     = 20
	stdoutHeadWindow = 8
)

// StderrDrainTimeoutS is how long to wait for the stderr drain to finish after
// the child dies, before classifying on whatever has arrived.
const StderrDrainTimeoutS = 1.0

// MaxRestarts is the number of restart attempts before a child is declared
// failed. radeontop exits immediately when it cannot use the card; retrying
// that forever would be a busy loop.
const MaxRestarts = 2

// failureSignal maps a substring radeontop prints to what to report. It writes
// some of them to stdout and some to stderr, so both streams are searched. The
// ordering matters: the permission message is checked first because it is the
// one a user can actually act on.
type failureSignal struct {
	needle string
	status model.BlocksStatus
}

var radeontopSignals = []failureSignal{
	{"cannot access gpu registers", model.BlocksNotPermitted},
	{"are you root", model.BlocksNotPermitted},
	{"failed to open drm node", model.BlocksNotPermitted},
	{"permission denied", model.BlocksNotPermitted},
	{"failed to find drm devices", model.BlocksUnsupported},
	{"unsupported bus type", model.BlocksUnsupported},
	{"failed to open", model.BlocksUnsupported},
	{"no vram support", model.BlocksUnsupported},
}

// RadeontopOptions is how to run radeontop.
type RadeontopOptions struct {
	// Enabled says whether the blocks panel may be shown at all. Off by
	// default: an unprivileged child answers "are you root?", so a default-on
	// panel would put a permanent complaint in every status line.
	Enabled bool
	// Binary is a program name looked up on $PATH, or an absolute path.
	Binary string
	// Ticks is the samples per second requested from radeontop.
	Ticks int
	// IntervalS is the whole seconds between dumps.
	IntervalS int
	// DrmRoot is the directory the DRM nodes live in, so -p can be preferred
	// to -b.
	DrmRoot string
	// Restart says whether a child that dies is restarted.
	Restart bool
}

// Validated clamps the options to ranges radeontop would accept anyway.
//
// Clamping rather than rejecting means a hand-written config with
// "interval_s: 0" produces a working program and a sane interval, instead of
// an error the user has to read a man page to resolve.
func (o RadeontopOptions) Validated() RadeontopOptions {
	out := o
	if out.Binary == "" {
		out.Binary = DefaultRadeontopBinary
	}
	if out.DrmRoot == "" {
		out.DrmRoot = DefaultDrmNodeRoot
	}
	out.Ticks = clampRadeontopInt(out.Ticks, 1, 10_000)
	out.IntervalS = clampRadeontopInt(out.IntervalS, MinDumpIntervalS, MaxDumpIntervalS)
	return out
}

// clampRadeontopInt floors v to [lo, hi].
func clampRadeontopInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// RadeontopDump is one parsed line of radeontop's dump output.
type RadeontopDump struct {
	// Seconds is the wall-clock seconds from the line's own timestamp.
	Seconds int
	// Microseconds is the microseconds within that second.
	Microseconds int
	// Bus is the PCI bus radeontop sampled, hexadecimal as printed.
	Bus string
	// Blocks is utilisation per block key, as a percentage.
	Blocks map[string]float64
	// Clocks maps a clock key to (percent, megahertz).
	Clocks map[string][2]float64
	// Memory maps a memory key to (percent, megabytes).
	Memory map[string][2]float64
}

// RestartResetS is how long a child must stay alive before it is considered to
// have proven itself and the restart budget is restored. Sized well above
// radeontop's own start-up so a child that can never work is not mistaken for
// one that merely took a while.

// ToBlocks converts to the model type the rest of the program passes around.
//
// nowNs is the sampler's current *wall-clock* time, used to record staleness.
// Left nil the sample claims no age, which is correct for a line the reader
// goroutine has only just parsed. The clock matters: dump.c stamps each line
// with gettimeofday() -- a wall-clock epoch -- while the rest of gputop
// measures intervals monotonically, a different origin. Passing a monotonic
// value here produced an age of about 56 years for every sample.
func (d RadeontopDump) ToBlocks(nowNs *model.Nanoseconds) model.GpuBlocks {
	blocks := make([]model.BlockReading, 0, len(model.BlockOrder))
	for _, key := range model.BlockOrder {
		if percent, ok := d.Blocks[key]; ok {
			blocks = append(blocks, model.BlockReading{
				Key: key, Label: model.BlockLabel(key), Percent: percent,
			})
		}
	}
	aux := make([]model.BlockReading, 0, len(model.AuxBlockOrder))
	for _, key := range model.AuxBlockOrder {
		if percent, ok := d.Blocks[key]; ok {
			aux = append(aux, model.BlockReading{
				Key: key, Label: model.BlockLabel(key), Percent: percent,
			})
		}
	}
	clocks := make([]model.ClockReading, 0, len(model.ClockKeys))
	for _, key := range model.ClockKeys {
		pair, ok := d.Clocks[key]
		if !ok {
			continue
		}
		percent := pair[0]
		mhz := model.Mhz(math.Round(pair[1]))
		clocks = append(clocks, model.ClockReading{
			Key: key, Label: model.BlockLabel(key), Percent: &percent, Mhz: &mhz,
		})
	}
	timestamp := model.Nanoseconds(d.Seconds)*1_000_000_000 + model.Nanoseconds(d.Microseconds)*1_000
	age := int64(0)
	if nowNs != nil {
		age = *nowNs - timestamp
		if age < 0 {
			age = 0
		}
	}
	return model.GpuBlocks{
		Blocks:      blocks,
		AuxBlocks:   aux,
		Clocks:      clocks,
		TimestampNs: timestamp,
		AgeNs:       age,
	}
}

// ParseDumpLine parses one line of "radeontop -d" output.
//
// The line is parsed by field name rather than by position, and that is not a
// stylistic choice. dump.c emits tc, smx, cr, uvd, vce0, vram, gtt and the
// clocks only when the card reports those bits, so a positional parser would
// read vgt's value as ta's on every modern card and report confidently wrong
// numbers. Naming the fields makes a missing block a missing block.
//
// A nil return is routine: radeontop writes its "Dumping to ..., until
// termination." banner to stdout with a plain printf, interleaved with the
// data, and a card it cannot read writes a sentence instead of a data line.
func ParseDumpLine(line string) *RadeontopDump {
	header := radeontopHeader.FindStringSubmatch(line)
	if header == nil {
		return nil
	}
	seconds, errSeconds := strconv.Atoi(header[1])
	microseconds, errMicros := strconv.Atoi(header[2])
	if errSeconds != nil || errMicros != nil {
		return nil
	}
	dump := &RadeontopDump{
		Seconds:      seconds,
		Microseconds: microseconds,
		Bus:          strings.ToLower(header[3]),
		Blocks:       map[string]float64{},
		Clocks:       map[string][2]float64{},
		Memory:       map[string][2]float64{},
	}
	for _, chunk := range strings.Split(header[4], ",") {
		field := radeontopField.FindStringSubmatch(strings.TrimSpace(chunk))
		if field == nil {
			continue
		}
		key, payload := field[1], field[2]
		numbers := radeontopNumber.FindAllString(payload, -1)
		if len(numbers) == 0 {
			continue
		}
		values := make([]float64, 0, len(numbers))
		for _, text := range numbers {
			value, err := strconv.ParseFloat(text, 64)
			if err != nil {
				value = 0
			}
			values = append(values, value)
		}
		switch {
		case hasKey(model.BlockLabels, key):
			dump.Blocks[key] = values[0]
		case containsString(model.MemoryKeys, key):
			// "vram 13.13% 4096.00mb" -- percentage then megabytes.
			megabytes := 0.0
			if len(values) > 1 {
				megabytes = values[1]
			}
			dump.Memory[key] = [2]float64{values[0], megabytes}
		case containsString(model.ClockKeys, key):
			// "sclk 25.00% 2.100ghz" -- percentage then gigahertz.
			gigahertz := 0.0
			if len(values) > 1 {
				gigahertz = values[1]
			}
			dump.Clocks[key] = [2]float64{values[0], gigahertz * 1000.0}
		}
	}
	return dump
}

// hasKey reports whether a string-keyed map holds key.
func hasKey[V any](m map[string]V, key string) bool {
	_, ok := m[key]
	return ok
}

// containsString reports whether list holds want.
func containsString(list []string, want string) bool {
	for _, item := range list {
		if item == want {
			return true
		}
	}
	return false
}

// BusFromBDF extracts the PCI bus from a domain:bus:device.function address.
//
// radeontop's -b takes the bus as it appears in lspci output, which is the
// two-digit hexadecimal field -- 0000:0c:00.0 gives "0c". It reports ok=false
// for a malformed address, which is a different answer from "bus 00".
func BusFromBDF(bdf string) (string, bool) {
	parts := strings.Split(bdf, ":")
	if len(parts) != 3 || parts[1] == "" {
		return "", false
	}
	bus := strings.ToLower(parts[1])
	for _, c := range bus {
		if !strings.ContainsRune("0123456789abcdef", c) {
			return "", false
		}
	}
	return bus, true
}

// BuildArgv builds the command line for one radeontop child.
//
// Dump mode is "-d - -i <interval> -l 0": stdout rather than a file, so the
// child's output can be consumed as a stream instead of re-read from disk, and
// a line limit of zero, which is radeontop's spelling of "until terminated".
// The node is preferred over the bus because the bus alone is ambiguous on a
// machine with two cards behind the same PCI bridge, and a wrong card's
// numbers would be worse than no numbers.
//
// The options are validated here, so a zero value yields radeontop's own
// defaults (-i 1, -t 120) rather than "-i 0 -t 0", which radeontop would
// silently reinterpret.
func BuildArgv(options RadeontopOptions, bus, node string) []string {
	options = options.Validated()
	argv := []string{
		options.Binary,
		"-d", "-",
		"-i", strconv.Itoa(options.IntervalS),
		"-l", "0",
		"-t", strconv.Itoa(options.Ticks),
	}
	if node != "" {
		argv = append(argv, "-p", node)
	} else if bus != "" {
		argv = append(argv, "-b", bus)
	}
	return argv
}

// ClassifyFailure maps captured diagnostic output onto a status, or ok=false
// when nothing matches.
//
// The argument is *both* streams, not stderr alone. radeontop splits its
// complaints: the packaged build writes "Cannot access GPU registers, are you
// root?" and "Failed to open DRM node" to stdout, and only "Failed to find DRM
// devices" to stderr. Reading only stderr therefore classifies a privilege
// problem as an unsupported card -- the least actionable of the available
// answers, and the one this function exists to avoid.
//
// Matching is safe against a stream that also carries data: a dump line is
// numbers, commas and a percent sign, and none of the signals below can occur
// in one.
func ClassifyFailure(lines []string) (model.BlocksStatus, bool) {
	blob := strings.ToLower(strings.Join(lines, " "))
	if strings.TrimSpace(blob) == "" {
		return "", false
	}
	for _, signal := range radeontopSignals {
		if strings.Contains(blob, signal.needle) {
			return signal.status, true
		}
	}
	return "", false
}

// ResolveBinary finds the radeontop executable.
//
// A name without a separator is looked up on $PATH; anything else is treated
// as a path and must be a file with an executable bit.
func ResolveBinary(name string) (string, bool) {
	if name == "" {
		return "", false
	}
	if filepath.IsAbs(name) || strings.ContainsRune(name, os.PathSeparator) {
		if isExecutableFile(name) {
			return name, true
		}
		return "", false
	}
	found, err := exec.LookPath(name)
	if err != nil {
		return "", false
	}
	return found, true
}

// isExecutableFile reports whether path is a regular file with an executable bit.
func isExecutableFile(path string) bool {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}
	return info.Mode().Perm()&0o111 != 0
}

// drmNode returns the DRM node for a card when one exists, else "".
//
// The existence check is what makes the fake sysfs trees in the test suite
// work without inventing DRM nodes.
func drmNode(options RadeontopOptions, card string) string {
	if card == "" {
		return ""
	}
	node := filepath.Join(options.DrmRoot, card)
	if info, err := os.Stat(node); err == nil && !info.IsDir() {
		return node
	}
	return ""
}

// radeontopChild is one live radeontop process and the pipes that drain it.
type radeontopChild struct {
	cmd    *exec.Cmd
	stdout io.ReadCloser
	stderr io.ReadCloser
	// done is closed once the process has been reaped.
	done chan struct{}
	// stderrDone is closed once the stderr drain has finished, so a failure
	// can be classified on the complete diagnostic rather than a partial one.
	stderrDone chan struct{}
	// readers counts the two drain goroutines.
	readers sync.WaitGroup
}

// alive reports whether the child is still running.
func (c *radeontopChild) alive() bool {
	select {
	case <-c.done:
		return false
	default:
		return true
	}
}

// closePipes closes this child's pipe ends.
//
// Closed explicitly rather than left to the garbage collector: the parent's
// ends of both pipes stay open until the process is reaped, and a session that
// starts and stops children repeatedly -- every restart, every device switch --
// would leak two descriptors each time and eventually run the monitor out of
// them.
func (c *radeontopChild) closePipes() {
	for _, pipe := range []io.ReadCloser{c.stdout, c.stderr} {
		if pipe != nil {
			_ = pipe.Close()
		}
	}
}

// RadeontopSource supervises one radeontop process, for one GPU.
//
// The one lock in this module: the sampler goroutine starts and polls the
// child, and teardown may run from another goroutine, so those two can
// genuinely race on process creation and teardown. The lock is held only
// across start/terminate/wait -- a handful of syscalls per session and per
// restart -- and never while reading a sample. The values the sampler reads
// (status, blocks) are published by rebinding, so the hot path keeps the
// lock-free property the rest of the program depends on.
type RadeontopSource struct {
	options RadeontopOptions
	key     int
	bus     string
	node    string

	mu        sync.Mutex
	child     *radeontopChild
	stopFlag  atomic.Bool
	restarts  int
	startedAt time.Time

	// stateMu guards the bounded diagnostic windows.
	stateMu    sync.Mutex
	stderr     []string
	stdoutHead []string
	sawDump    bool

	status atomic.Pointer[model.BlocksStatus]
	blocks atomic.Pointer[model.GpuBlocks]
}

// NewRadeontopSource returns a source for one device. node is preferred over
// bus when both are set.
func NewRadeontopSource(options RadeontopOptions, key int, bus, node string) *RadeontopSource {
	source := &RadeontopSource{options: options.Validated(), key: key, bus: bus, node: node}
	source.setStatus(model.BlocksMissing)
	empty := model.GpuBlocks{}
	source.blocks.Store(&empty)
	return source
}

// Key is the device index this source serves.
func (s *RadeontopSource) Key() int { return s.key }

// Status is why this source is or is not producing data.
func (s *RadeontopSource) Status() model.BlocksStatus {
	if pointer := s.status.Load(); pointer != nil {
		return *pointer
	}
	return model.BlocksMissing
}

// Blocks is the most recent sample, empty when nothing has been parsed yet.
func (s *RadeontopSource) Blocks() model.GpuBlocks {
	if pointer := s.blocks.Load(); pointer != nil {
		return *pointer
	}
	return model.GpuBlocks{}
}

func (s *RadeontopSource) setStatus(status model.BlocksStatus) { s.status.Store(&status) }

func (s *RadeontopSource) setBlocks(blocks model.GpuBlocks) { s.blocks.Store(&blocks) }

// Start launches the child, or records why it could not be launched. It is
// safe to call more than once; a source already running is left alone.
func (s *RadeontopSource) Start() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.child != nil && s.child.alive() {
		return
	}
	binary, ok := ResolveBinary(s.options.Binary)
	if !ok {
		s.setStatus(model.BlocksMissing)
		return
	}
	argv := BuildArgv(s.options, s.bus, s.node)
	// argv[0] is the path that was just checked executable, so the child is
	// launched from exactly the file the resolution accepted.
	argv[0] = binary
	// argv is built from validated options, never passed through a shell.
	cmd := exec.Command(argv[0], argv[1:]...)
	// A new session, so a terminal interrupt reaches the child too rather than
	// leaving it orphaned and still holding the GPU.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		s.setStatus(model.BlocksFailed)
		return
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		s.setStatus(model.BlocksFailed)
		return
	}
	if err := cmd.Start(); err != nil {
		s.setStatus(model.BlocksFailed)
		return
	}
	// The previous child's pipes are closed before it is replaced. Restarting
	// is ordinary -- a driver reload or a hot-unplugged eGPU causes it -- and
	// without this every restart would leave two descriptors open.
	if previous := s.child; previous != nil {
		previous.closePipes()
	}
	child := &radeontopChild{
		cmd:        cmd,
		stdout:     stdout,
		stderr:     stderr,
		done:       make(chan struct{}),
		stderrDone: make(chan struct{}),
	}
	s.child = child
	s.startedAt = time.Now()
	// Everything below belongs to the child being replaced, not to this one.
	// Carrying sawDump over made every complaint the new child writes before
	// its first sample get discarded, so a replacement that cannot reach the
	// GPU reported "failed" instead of the reason it printed.
	s.stopFlag.Store(false)
	s.resetCapture()
	s.setStatus(model.BlocksOK)
	child.readers.Add(2)
	go s.readStdout(child)
	go s.readStderr(child)
	go func() {
		child.readers.Wait()
		_ = cmd.Wait()
		close(child.done)
	}()
}

// Stop terminates the child and lets its readers finish. It is idempotent.
//
// Termination is polite first: radeontop installs a SIGTERM handler so it can
// flush the frame it is writing. It is killed only if it does not go quietly,
// because a child that ignores SIGTERM would otherwise outlive the monitor
// that started it.
func (s *RadeontopSource) Stop() {
	s.mu.Lock()
	s.stopFlag.Store(true)
	child := s.child
	s.child = nil
	s.mu.Unlock()
	if child == nil {
		return
	}
	if child.cmd.Process != nil {
		_ = child.cmd.Process.Signal(syscall.SIGTERM)
		select {
		case <-child.done:
		case <-time.After(2 * time.Second):
			_ = child.cmd.Process.Kill()
			select {
			case <-child.done:
			case <-time.After(2 * time.Second):
			}
		}
	}
	child.closePipes()
}

// -- drain loops -----------------------------------------------------------

// readStdout parses dump lines until the child closes stdout or the source is
// stopped.
func (s *RadeontopSource) readStdout(child *radeontopChild) {
	defer child.readers.Done()
	scanner := bufio.NewScanner(child.stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for scanner.Scan() {
		// A reader can outlive the child it was spawned for: a restart installs
		// a replacement while the previous child's stdout is still reaching
		// EOF. Everything below would then describe the child that is running
		// now, including marking a perfectly healthy one as failed.
		if s.stopped() || !s.isCurrent(child) {
			return
		}
		sample := ParseDumpLine(scanner.Text())
		if sample != nil {
			// From here on stdout is data; anything before this was a complaint.
			s.markDumpSeen()
			s.setBlocks(sample.ToBlocks(nil))
			continue
		}
		if !s.dumpSeen() {
			s.appendStdoutHead(strings.TrimSpace(scanner.Text()))
		}
	}
	s.onChildExit(child)
}

// readStderr retains stderr so a failure can be explained rather than merely
// reported.
func (s *RadeontopSource) readStderr(child *radeontopChild) {
	defer child.readers.Done()
	defer close(child.stderrDone)
	scanner := bufio.NewScanner(child.stderr)
	scanner.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for scanner.Scan() {
		s.appendStderr(strings.TrimSpace(scanner.Text()))
	}
}

// onChildExit classifies why a child stopped producing output.
//
// The stderr drain is awaited first, and that ordering is load-bearing rather
// than tidy. radeontop reports *every* problem it has, most usefully last: it
// prints "Failed to find DRM devices", then "Failed to open DRM node", then
// finally "Cannot access GPU registers, are you root?". stdout closes as soon
// as the child dies while stderr is still being delivered, so classifying on
// stdout EOF alone reads a partial buffer and reports the least actionable of
// the three -- telling a user their card is unsupported when the real answer
// is that they need root. The join is bounded, because a child that exits
// while leaving its stderr pipe open would otherwise hang the sampler thread.
func (s *RadeontopSource) onChildExit(child *radeontopChild) {
	if s.stopped() || !s.isCurrent(child) {
		return
	}
	select {
	case <-child.stderrDone:
	case <-time.After(time.Duration(StderrDrainTimeoutS * float64(time.Second))):
	}
	if status, ok := ClassifyFailure(s.capturedLines()); ok {
		s.setStatus(status)
		return
	}
	s.setStatus(model.BlocksFailed)
}

// Poll re-checks liveness, restarting the child when that is allowed.
//
// Called from the sampler goroutine on every tick. A child that exits during a
// normal session -- a driver reload, a hot-unplugged eGPU -- should not
// permanently silence the panel, so it is restarted a bounded number of times
// and then left failed.
func (s *RadeontopSource) Poll() model.BlocksStatus {
	if s.stopped() {
		return s.Status()
	}
	s.mu.Lock()
	child := s.child
	if child != nil && child.alive() {
		// A child that has outlived the reset window has proven itself, so the
		// next failure it suffers gets the full budget again. Without this a
		// long session accumulates restarts towards the cap over hours and ends
		// up unable to recover from a single ordinary hiccup.
		if time.Since(s.startedAt) >= time.Duration(RestartResetS*float64(time.Second)) {
			s.restarts = 0
		}
		s.mu.Unlock()
		return s.Status()
	}
	s.mu.Unlock()
	if child != nil {
		s.onChildExit(child)
	}
	status := s.Status()
	if status == model.BlocksOK || !s.options.Restart {
		return status
	}
	// Only a transient failure is worth retrying; a missing binary or a
	// refused ioctl will refuse again just as promptly.
	if status == model.BlocksMissing || status == model.BlocksNotPermitted ||
		status == model.BlocksUnsupported {
		return status
	}
	s.mu.Lock()
	exhausted := s.restarts >= MaxRestarts
	if !exhausted {
		s.restarts++
	}
	s.mu.Unlock()
	if exhausted {
		s.setStatus(model.BlocksFailed)
		return model.BlocksFailed
	}
	s.Start()
	return s.Status()
}

// stopped reports whether this source has been stopped.
func (s *RadeontopSource) stopped() bool { return s.stopFlag.Load() }

// isCurrent reports whether child is still the child this source owns.
func (s *RadeontopSource) isCurrent(child *radeontopChild) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.child == child
}

// -- bounded diagnostic capture --------------------------------------------

func (s *RadeontopSource) resetCapture() {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	s.stderr = nil
	s.stdoutHead = nil
	s.sawDump = false
}

func (s *RadeontopSource) markDumpSeen() {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	s.sawDump = true
}

func (s *RadeontopSource) dumpSeen() bool {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	return s.sawDump
}

func (s *RadeontopSource) appendStdoutHead(line string) {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	s.stdoutHead = appendBounded(s.stdoutHead, line, stdoutHeadWindow)
}

func (s *RadeontopSource) appendStderr(line string) {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	s.stderr = appendBounded(s.stderr, line, stderrWindow)
}

// capturedLines returns the retained stdout head followed by stderr, which is
// what a failure is classified from.
func (s *RadeontopSource) capturedLines() []string {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	out := make([]string, 0, len(s.stdoutHead)+len(s.stderr))
	out = append(out, s.stdoutHead...)
	out = append(out, s.stderr...)
	return out
}

// stderrText returns the retained stderr lines, for a diagnostic message.
func (s *RadeontopSource) stderrText() string {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	return strings.Join(s.stderr, " ")
}

// appendBounded appends line, keeping at most limit entries.
func appendBounded(window []string, line string, limit int) []string {
	window = append(window, line)
	if len(window) > limit {
		window = window[len(window)-limit:]
	}
	return window
}

// RadeontopPool is one RadeontopSource per GPU, plus the panel's overall
// verdict.
//
// The pool exists so the rest of the program deals with a single object: the
// sampler creates one, calls Poll each tick, and asks it for a status and a
// per-device sample. It does not need to know that a subprocess is involved,
// or that there may be several. It implements sampler.BlocksSource.
type RadeontopPool struct {
	options RadeontopOptions
	sources map[int]*RadeontopSource
}

// NewRadeontopPool returns a pool with validated options and no sources.
func NewRadeontopPool(options RadeontopOptions) *RadeontopPool {
	return &RadeontopPool{
		options: options.Validated(),
		sources: map[int]*RadeontopSource{},
	}
}

// Options returns the validated options in effect.
func (p *RadeontopPool) Options() RadeontopOptions { return p.options }

// Enabled reports whether the blocks panel may be shown.
func (p *RadeontopPool) Enabled() bool { return p.options.Enabled }

// Configure creates one source per device, in the sampler's device order.
func (p *RadeontopPool) Configure(devices []model.AmdgpuDevice) {
	p.Stop()
	p.sources = map[int]*RadeontopSource{}
	if !p.options.Enabled {
		return
	}
	for _, device := range devices {
		bus, _ := BusFromBDF(device.BDF)
		p.sources[device.Index] = NewRadeontopSource(
			p.options, device.Index, bus, drmNode(p.options, device.Card),
		)
	}
}

// Start starts every source.
func (p *RadeontopPool) Start() {
	if !p.options.Enabled {
		return
	}
	for _, source := range p.sources {
		source.Start()
	}
}

// Stop stops every source. It is idempotent.
func (p *RadeontopPool) Stop() {
	for _, source := range p.sources {
		source.Stop()
	}
}

// Status is the worst status across every source.
//
// "Worst" is ordered by how actionable it is rather than by severity: a card
// that radeontop cannot read at all is reported as unsupported even if another
// card works, because a panel that silently covers only half the GPUs is the
// more misleading of the two outcomes.
func (p *RadeontopPool) Status() model.BlocksStatus {
	if !p.options.Enabled {
		return model.BlocksDisabled
	}
	if len(p.sources) == 0 {
		return model.BlocksMissing
	}
	present := map[model.BlocksStatus]bool{}
	for _, source := range p.sources {
		present[source.Status()] = true
	}
	for _, candidate := range []model.BlocksStatus{
		model.BlocksNotPermitted,
		model.BlocksMissing,
		model.BlocksUnsupported,
		model.BlocksFailed,
	} {
		if present[candidate] {
			return candidate
		}
	}
	return model.BlocksOK
}

// BlocksFor returns the latest sample for a device index, or nil when there is
// none.
//
// nil and an empty GpuBlocks are different answers: the first means the child
// has not produced a line for this card yet, the second means it produced one
// that had no blocks in it.
func (p *RadeontopPool) BlocksFor(index int) *model.GpuBlocks {
	source := p.sources[index]
	if source == nil || source.Status() != model.BlocksOK {
		return nil
	}
	blocks := source.Blocks()
	if blocks.IsEmpty() {
		return nil
	}
	return &blocks
}

// Poll refreshes every source and returns the overall status.
func (p *RadeontopPool) Poll() model.BlocksStatus {
	if !p.options.Enabled {
		return model.BlocksDisabled
	}
	for _, source := range p.sources {
		source.Poll()
	}
	return p.Status()
}

// Diagnostic returns a one-line explanation of the current status, or "" when
// healthy.
func (p *RadeontopPool) Diagnostic() string {
	if hint := p.Status().Hint(); hint != "" {
		return hint
	}
	for _, source := range p.sources {
		if text := source.stderrText(); text != "" {
			return text
		}
	}
	return ""
}

// WaitForFirstSample blocks until every source has produced a line, or the
// timeout expires.
//
// Only used by the one-shot CLI paths, which have no sampler goroutine to wait
// on: a --dump needs the panel to have real numbers the first time it renders.
func WaitForFirstSample(pool *RadeontopPool, timeout time.Duration) bool {
	if len(pool.sources) == 0 {
		return false
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		pool.Poll()
		complete := true
		for index := range pool.sources {
			if pool.BlocksFor(index) == nil {
				complete = false
				break
			}
		}
		if complete {
			return true
		}
		time.Sleep(50 * time.Millisecond)
	}
	return false
}

const RestartResetS = 60.0
