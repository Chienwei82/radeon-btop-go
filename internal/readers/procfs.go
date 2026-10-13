package readers

import (
	"os"
	"os/user"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"syscall"

	"github.com/gputop/gputop-go/internal/model"
)

// DRMDir is the prefix a GPU descriptor's target lives under. Matching the
// substring rather than a suffix matters: the target is
// /dev/dri/renderD128, so a suffix test against "/dri/renderD" can never
// match. One prefix test covers every DRM node, and the parser still insists
// the driver is amdgpu.
const DRMDir = "/dev/dri/"

var (
	engineActiveRe = regexp.MustCompile(`^drm-engine-active-([a-z0-9_]+)$`)
	enginePeriodRe = regexp.MustCompile(`^drm-engine-period-([a-z0-9_]+)$`)
	engineRe       = regexp.MustCompile(`^drm-engine-([a-z0-9_]+)$`)
	// memoryRe matches the whole drm-{total,shared,resident,purgeable,memory}-*
	// family plus the AMD-specific amd-requested-*/amd-evicted-*, so a new pool
	// name added by the driver is captured without a code change.
	memoryRe = regexp.MustCompile(
		`^(?:drm-(?:total|shared|resident|purgeable|memory)|amd-requested|amd-evicted)-([a-z]+)$`)
)

// engineBuckets maps driver engine names onto display buckets. Anything
// unrecognised becomes "other", so a new engine still shows up instead of
// vanishing.
var engineBuckets = map[string]string{
	"gfx": "gfx", "compute": "compute",
	"sdma0": "dma", "sdma1": "dma", "sdma": "dma", "dma": "dma",
	"vangogh": "enc", "vcnenc": "enc", "venc": "enc", "vcn0.enc": "enc", "encode": "enc",
	"jpeg": "dec", "vdec": "dec", "vdpdec": "dec", "uvd": "dec",
	"va": "dec", "decode": "dec", "vcn0.dec": "dec",
	"video": "other", "vcn": "other", "vcn0": "other",
}

// NormaliseEngine maps a driver engine name onto a display bucket.
func NormaliseEngine(rawName string) string {
	name := strings.ToLower(strings.TrimSpace(rawName))
	if bucket, ok := engineBuckets[name]; ok {
		return bucket
	}
	if strings.HasPrefix(name, "sdma") || strings.HasPrefix(name, "dma") {
		return "dma"
	}
	if strings.Contains(name, "enc") {
		return "enc"
	}
	if strings.Contains(name, "dec") || strings.Contains(name, "jpeg") {
		return "dec"
	}
	return "other"
}

// FdinfoRecord is one parsed fdinfo file for a DRM file descriptor.
type FdinfoRecord struct {
	Driver   string
	ClientID model.ClientId
	Pdev     model.Bdf
	Engines  map[string]int64
	Memory   map[string]model.Bytes
}

// leadingInt extracts the leading integer from a value that may carry a unit.
// The amdgpu driver writes engine counters as "173321 ns", so parsing the
// whole string with strconv would fail and take the sample down with it.
func leadingInt(value string) (int64, bool) {
	fields := strings.Fields(value)
	if len(fields) == 0 {
		return 0, false
	}
	v, err := strconv.ParseInt(fields[0], 10, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

// ParseFdinfo parses the DRM accounting keys out of one fdinfo file. It
// returns nil when the file is not DRM accounting data or belongs to another
// driver: there is no vendor abstraction layer here.
func ParseFdinfo(text string) *FdinfoRecord {
	var driver string
	var clientID *model.ClientId
	var pdev model.Bdf
	engines := map[string]int64{}
	active := map[string]int64{}
	period := map[string]int64{}
	memory := map[string]model.Bytes{}

	for _, line := range strings.Split(text, "\n") {
		key, value, sep := strings.Cut(line, ":")
		if !sep {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)

		switch {
		case key == "drm-driver":
			driver = value
		case key == "drm-client-id":
			if v, ok := leadingInt(value); ok {
				id := model.ClientId(v)
				clientID = &id
			}
		case key == "drm-pdev":
			pdev = value
		case strings.HasPrefix(key, "drm-engine-active-"):
			if m := engineActiveRe.FindStringSubmatch(key); m != nil {
				if v, ok := leadingInt(value); ok {
					active[m[1]] = v
				}
			}
		case strings.HasPrefix(key, "drm-engine-period-"):
			if m := enginePeriodRe.FindStringSubmatch(key); m != nil {
				if v, ok := leadingInt(value); ok {
					period[m[1]] = v
				}
			}
		case strings.HasPrefix(key, "drm-engine-"):
			if m := engineRe.FindStringSubmatch(key); m != nil {
				if v, ok := leadingInt(value); ok {
					engines[m[1]] = v
				}
			}
		case memoryRe.MatchString(key):
			if size, ok := ParseDrmSize(value); ok {
				memory[key] = size
			}
		}
	}

	if driver == "" || clientID == nil || driver != "amdgpu" {
		return nil
	}

	// amdgpu emits only drm-engine-<name>. The active/period pair is accepted
	// defensively, for a driver that reports a busy window directly rather
	// than a cumulative total; it wins when present because it needs no
	// baseline.
	for name, busy := range active {
		if window := period[name]; window > 0 {
			engines[name] = busy
		}
	}
	return &FdinfoRecord{
		Driver: driver, ClientID: *clientID, Pdev: pdev,
		Engines: engines, Memory: memory,
	}
}

// clientKey is the kernel's own dedup pair.
type clientKey struct {
	bdf    model.Bdf
	client model.ClientId
}

// clientAcc is mutable scratch state used only while merging one scan.
type clientAcc struct {
	pid      int
	name     string
	user     string
	bdf      model.Bdf
	clientID model.ClientId
	cmdline  []string
	ppid     *int
	engines  map[string]int64
	memory   map[string]model.Bytes
}

// ProcessCollector scans a proc tree for DRM clients and diffs their
// cumulative engine counters against the previous scan.
//
// The scan is the only part of a sample whose cost grows with the size of the
// system rather than with the number of GPUs: it visits every process and
// reads one symlink per open descriptor. Paths are therefore built from
// strings, never from path-like objects.
type ProcessCollector struct {
	procRoot string
	// previous holds per-client cumulative engine counters from the previous
	// scan. One window for the whole scan, so a single timestamp is enough.
	previous map[clientKey]map[string]int64
	// lastScanNs is nil rather than 0 to mean "no sample yet": a timestamp of
	// exactly 0 is a legitimate value, so it cannot double as the flag.
	lastScanNs *model.Nanoseconds
	// users caches uid -> name: the lookup is an NSS call, and a scan would
	// otherwise repeat it for every client on every tick.
	users map[uint32]string
}

// NewProcessCollector returns a collector rooted at procRoot ("" means /proc).
func NewProcessCollector(procRoot string) *ProcessCollector {
	if procRoot == "" {
		procRoot = "/proc"
	}
	return &ProcessCollector{
		procRoot: procRoot,
		previous: map[clientKey]map[string]int64{},
		users:    map[uint32]string{},
	}
}

// Reset forgets the delta baseline, e.g. after a device rescan.
func (c *ProcessCollector) Reset() {
	c.previous = map[clientKey]map[string]int64{}
	c.lastScanNs = nil
}

// Collect scans the proc tree and returns the current DRM clients plus the
// visible/total process counts. total is the number of process directories
// seen; visible the number whose descriptors could actually be inspected. The
// two differing is what reveals that /proc permissions hid other users'
// processes, which the UI reports rather than hiding. visible counts
// *inspectable* processes, not ones holding a DRM descriptor: a plain process
// is perfectly visible and must not be reported as hidden.
func (c *ProcessCollector) Collect(nowNs model.Nanoseconds) ([]model.GpuProcess, int, int) {
	clients, total, visible := c.scanClients()
	return c.buildProcesses(clients, nowNs), visible, total
}

// scanClients walks the proc tree and merges every DRM record by
// (drm-pdev, drm-client-id).
func (c *ProcessCollector) scanClients() (map[clientKey]*clientAcc, int, int) {
	clients := map[clientKey]*clientAcc{}
	entries, err := os.ReadDir(c.procRoot)
	if err != nil {
		return clients, 0, 0
	}
	total, visible := 0, 0
	for _, entry := range entries {
		if !isDigits(entry.Name()) {
			continue
		}
		total++
		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}
		pidDir := filepath.Join(c.procRoot, entry.Name())
		records, ok := readPID(pidDir)
		if !ok {
			// Could not inspect this process at all: another user's
			// descriptor directory, or the process exited between the
			// scan and the read.
			continue
		}
		visible++
		if len(records) > 0 {
			c.merge(clients, pid, pidDir, records)
		}
	}
	return clients, total, visible
}

// isDigits reports whether a directory name is a PID.
func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// readPID returns every DRM fdinfo record held by one process, and whether the
// process could be inspected at all. An unreadable fd directory is the
// ordinary state of another user's process without root; the distinction
// between "hidden" and "holds no GPU descriptor" is what the visible count is
// made of.
func readPID(pidDir string) ([]*FdinfoRecord, bool) {
	fdEntries, err := os.ReadDir(filepath.Join(pidDir, "fd"))
	if err != nil {
		return nil, false
	}
	var records []*FdinfoRecord
	for _, fd := range fdEntries {
		// The descriptor's target is what identifies a GPU client, so every
		// open descriptor costs one readlink.
		target, err := os.Readlink(filepath.Join(pidDir, "fd", fd.Name()))
		if err != nil {
			continue
		}
		if !strings.Contains(target, DRMDir) {
			continue
		}
		// An ENOENT here means the same thing as a missing fdinfo entry, so no
		// extra listing of the fdinfo directory is needed.
		text, ok := ReadText(filepath.Join(pidDir, "fdinfo", fd.Name()), TableLimit)
		if !ok {
			continue
		}
		if rec := ParseFdinfo(text); rec != nil {
			records = append(records, rec)
		}
	}
	return records, true
}

// merge folds one process's records into the shared per-client accumulators.
func (c *ProcessCollector) merge(
	clients map[clientKey]*clientAcc, pid int, pidDir string, records []*FdinfoRecord,
) {
	name, userName, ppid, cmdline := processIdentity(pidDir, c.users)
	for _, rec := range records {
		key := clientKey{rec.Pdev, rec.ClientID}
		acc := clients[key]
		if acc == nil {
			acc = &clientAcc{
				pid: pid, name: name, user: userName, bdf: rec.Pdev,
				clientID: rec.ClientID, cmdline: cmdline, ppid: ppid,
				engines: map[string]int64{}, memory: map[string]model.Bytes{},
			}
			clients[key] = acc
		} else if pid < acc.pid {
			// Prefer the lowest PID so the row is stable across scans.
			acc.pid, acc.name, acc.user = pid, name, userName
			acc.cmdline, acc.ppid = cmdline, ppid
		}
		// Maximum, never sum: two fds onto one client report the same totals.
		for engine, totalNs := range rec.Engines {
			if prev, ok := acc.engines[engine]; !ok || totalNs > prev {
				acc.engines[engine] = totalNs
			}
		}
		for name, size := range rec.Memory {
			if prev, ok := acc.memory[name]; !ok || size > prev {
				acc.memory[name] = size
			}
		}
	}
}

// buildProcesses turns accumulators into rows with delta-derived engine usage.
func (c *ProcessCollector) buildProcesses(
	clients map[clientKey]*clientAcc, nowNs model.Nanoseconds,
) []model.GpuProcess {
	var windowNs model.Nanoseconds
	if c.lastScanNs != nil {
		windowNs = nowNs - *c.lastScanNs
		if windowNs < 0 {
			windowNs = 0
		}
	}
	processes := make([]model.GpuProcess, 0, len(clients))
	current := make(map[clientKey]map[string]int64, len(clients))

	for key, acc := range clients {
		previous := c.previous[key]
		usage := make([]model.EngineUsage, 0, len(acc.engines))
		for _, engine := range sortedEngineNames(acc.engines) {
			totalNs := acc.engines[engine]
			baseline, hasBaseline := previous[engine]
			var delta model.Nanoseconds
			if hasBaseline && totalNs >= baseline {
				delta = totalNs - baseline
			}
			// Without a baseline there is no window either: the percentage
			// must read as unknown, not as zero.
			var rowWindow model.Nanoseconds
			if hasBaseline {
				rowWindow = windowNs
			}
			usage = append(usage, model.EngineUsage{
				Engine: NormaliseEngine(engine), TotalNs: totalNs,
				DeltaNs: delta, WindowNs: rowWindow,
			})
		}
		current[key] = acc.engines
		processes = append(processes, model.GpuProcess{
			PID: acc.pid, Name: acc.name, User: acc.user, BDF: acc.bdf,
			ClientID: acc.clientID, Cmdline: acc.cmdline, PPID: acc.ppid,
			Engines:    usage,
			VramUsed:   preferredMemory(acc.memory, "drm-resident-vram", "drm-total-vram"),
			VramShared: memoryAt(acc.memory, "drm-shared-vram"),
			GttUsed:    preferredMemory(acc.memory, "drm-resident-gtt", "drm-total-gtt"),
			GttShared:  memoryAt(acc.memory, "drm-shared-gtt"),
			CpuUsed:    preferredMemory(acc.memory, "drm-resident-cpu", "drm-total-cpu"),
		})
	}

	c.previous = current
	stamp := nowNs
	c.lastScanNs = &stamp
	sortProcesses(processes)
	return processes
}

// sortedEngineNames returns a map's keys in a deterministic order.
func sortedEngineNames(m map[string]int64) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// preferredMemory returns resident memory when the kernel provides it, else
// the total, else nil.
func preferredMemory(memory map[string]model.Bytes, first, second string) *model.Bytes {
	if v, ok := memory[first]; ok {
		return &v
	}
	if v, ok := memory[second]; ok {
		return &v
	}
	return nil
}

// memoryAt returns one memory key, or nil when absent.
func memoryAt(memory map[string]model.Bytes, key string) *model.Bytes {
	if v, ok := memory[key]; ok {
		return &v
	}
	return nil
}

// sortProcesses orders rows busiest-first, breaking ties by memory and then
// PID for stability.
func sortProcesses(processes []model.GpuProcess) {
	sort.SliceStable(processes, func(i, j int) bool {
		return lessBusy(processes[i], processes[j])
	})
}

// lessBusy reports whether a should sort before b.
func lessBusy(a, b model.GpuProcess) bool {
	if a.EnginePercent() != b.EnginePercent() {
		return a.EnginePercent() > b.EnginePercent()
	}
	if a.MemoryUsed() != b.MemoryUsed() {
		return a.MemoryUsed() > b.MemoryUsed()
	}
	return a.PID < b.PID
}

// processIdentity returns (name, user, ppid, cmdline) for a process, tolerating
// a vanished one.
//
// status is read because PPid lives there and only there, and the process tree
// is built from it. It is read for processes that hold a DRM descriptor and
// for no others, so the extra read is bounded by the size of the table rather
// than by the size of the proc tree. comm is preferred for the name because it
// needs no parsing.
func processIdentity(pidDir string, users map[uint32]string) (string, string, *int, []string) {
	name := ""
	var ppid *int
	if text, ok := ReadText(filepath.Join(pidDir, "comm"), ValueLimit); ok {
		name = text
	}
	if text, ok := ReadText(filepath.Join(pidDir, "status"), ValueLimit); ok {
		for _, line := range strings.Split(text, "\n") {
			switch {
			case name == "" && strings.HasPrefix(line, "Name:"):
				name = strings.TrimSpace(strings.TrimPrefix(line, "Name:"))
			case strings.HasPrefix(line, "PPid:"):
				if v, ok := leadingInt(strings.TrimPrefix(line, "PPid:")); ok {
					parent := int(v)
					ppid = &parent
				}
			}
		}
	}
	if name == "" {
		name = filepath.Base(pidDir)
	}

	info, err := os.Stat(pidDir)
	if err != nil {
		return name, "?", ppid, readCmdline(pidDir)
	}
	uid, ok := uidOf(info)
	if !ok {
		return name, "?", ppid, readCmdline(pidDir)
	}
	if cached, ok := users[uid]; ok {
		return name, cached, ppid, readCmdline(pidDir)
	}
	userName := "?"
	if u, err := user.LookupId(strconv.FormatUint(uint64(uid), 10)); err == nil {
		userName = u.Username
	}
	users[uid] = userName
	return name, userName, ppid, readCmdline(pidDir)
}

// uidOf extracts the owning uid from a stat result.
func uidOf(info os.FileInfo) (uint32, bool) {
	if st, ok := info.Sys().(*syscall.Stat_t); ok {
		return st.Uid, true
	}
	return 0, false
}

// readCmdline reads a process's cmdline as a list of arguments.
//
// The file is NUL-separated with a trailing NUL, and it is empty for a kernel
// thread -- which is exactly how the caller recognises one without root. An
// unreadable file is indistinguishable from an empty one here, and both mean
// "no argument vector", so an empty slice is the right answer either way.
func readCmdline(pidDir string) []string {
	raw, err := os.ReadFile(filepath.Join(pidDir, "cmdline"))
	if err != nil {
		return nil
	}
	var out []string
	for _, part := range strings.Split(string(raw), "\x00") {
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}
