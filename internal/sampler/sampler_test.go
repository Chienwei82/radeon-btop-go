package sampler

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gputop/gputop-go/internal/model"
)

// -- fixtures -----------------------------------------------------------

// v13 is the amdgpu gpu_metrics v1.3 table the reference Navi 21 emits.
const (
	v13StructureSize = 120
)

// v13Offsets are field byte offsets verified against a real Navi 21 table.
// The tests assert against these literal numbers so a bug in the aligner
// cannot silently rewrite the expectation along with the code.
var v13Offsets = map[string]int{
	"temperature_edge":      4,
	"temperature_hotspot":   6,
	"temperature_mem":       8,
	"average_gfx_activity":  16,
	"average_socket_power":  22,
	"energy_accumulator":    24,
	"system_clock_counter":  32,
	"current_gfxclk":        54,
	"current_uclk":          58,
	"throttle_status":       68,
	"current_fan_speed":     72,
	"pcie_link_width":       74,
	"pcie_link_speed":       76,
	"temperature_hbm":       88,
	"firmware_timestamp":    96,
	"voltage_soc":           104,
	"voltage_gfx":           106,
	"voltage_mem":           108,
	"indep_throttle_status": 112,
}

// buildV13 packs a v1.3 metric table with the given field overrides; every
// other field is zero.
func buildV13(values map[string]int) []byte {
	raw := make([]byte, v13StructureSize)
	for name, value := range values {
		offset := v13Offsets[name]
		binary.LittleEndian.PutUint16(raw[offset:], uint16(value))
	}
	// Header: <HBB structure_size, format_revision, content_revision.
	binary.LittleEndian.PutUint16(raw[0:], v13StructureSize)
	raw[2] = 1
	raw[3] = 3
	return raw
}

// drmFixture builds an empty fake /sys/class/drm plus the fake PCI root the
// card/device symlinks resolve into, mirroring the real layout.
func drmFixture(t *testing.T) (drmRoot, pciRoot string) {
	t.Helper()
	base := t.TempDir()
	drmRoot = filepath.Join(base, "sys", "class", "drm")
	pciRoot = filepath.Join(base, "devices", "pci0000:00", "0000:00:00.0")
	if err := os.MkdirAll(drmRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(pciRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	return drmRoot, pciRoot
}

// procFixture builds an empty fake /proc.
func procFixture(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "proc")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	return root
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// gpuSpec describes one synthetic GPU.
type gpuSpec struct {
	card       int
	bdf        string
	deviceID   int
	metrics    map[string]int
	noMetrics  bool
	extraSysfs map[string]string
}

// makeGpu writes one fake amdgpu GPU under drmRoot and returns its resolved
// device directory. It reproduces the cardN/device symlink so discovery can
// derive the BDF, and binds a driver link naming amdgpu.
func makeGpu(t *testing.T, drmRoot, pciRoot string, spec gpuSpec) string {
	t.Helper()
	if spec.bdf == "" {
		spec.bdf = "0000:0c:00.0"
	}
	if spec.deviceID == 0 {
		spec.deviceID = 0x73BF
	}
	pciDir := filepath.Join(pciRoot, spec.bdf)
	write(t, filepath.Join(pciDir, "boot_vga"), "1\n")
	write(t, filepath.Join(pciDir, "vendor"), "0x1002\n")
	write(t, filepath.Join(pciDir, "device"), fmt.Sprintf("0x%04x\n", spec.deviceID))
	driverLink := filepath.Join(pciDir, "driver")
	_ = os.Remove(driverLink)
	if err := os.Symlink("../../../../bus/pci/drivers/amdgpu", driverLink); err != nil {
		t.Fatal(err)
	}
	if !spec.noMetrics && spec.metrics != nil {
		write(t, filepath.Join(pciDir, "gpu_metrics"), string(buildV13(spec.metrics)))
	}
	for name, value := range spec.extraSysfs {
		write(t, filepath.Join(pciDir, name), value)
	}
	cardDir := filepath.Join(drmRoot, fmt.Sprintf("card%d", spec.card))
	if err := os.MkdirAll(cardDir, 0o755); err != nil {
		t.Fatal(err)
	}
	devLink := filepath.Join(cardDir, "device")
	_ = os.Remove(devLink)
	if err := os.Symlink(pciDir, devLink); err != nil {
		t.Fatal(err)
	}
	return pciDir
}

const (
	testPdev   = "0000:0c:00.0"
	testNode   = "/dev/dri/renderD128"
	testClient = "42"
)

// procSpec is one fake process.
type procSpec struct {
	name    string
	client  string
	pdev    string
	ppid    int
	cmdline []string
	noDRM   bool
	fdinfo  []string
}

// makeProc writes a fake process holding one DRM descriptor.
func makeProc(t *testing.T, procRoot string, pid int, spec procSpec) {
	t.Helper()
	if spec.name == "" {
		spec.name = "testproc"
	}
	if spec.client == "" {
		spec.client = testClient
	}
	if spec.pdev == "" {
		spec.pdev = testPdev
	}
	if spec.ppid == 0 {
		spec.ppid = 1
	}
	dir := filepath.Join(procRoot, fmt.Sprint(pid))
	write(t, filepath.Join(dir, "comm"), spec.name+"\n")
	write(t, filepath.Join(dir, "status"),
		fmt.Sprintf("Name:\t%s\nPPid:\t%d\n", spec.name, spec.ppid))
	argv := spec.cmdline
	if argv == nil {
		argv = []string{spec.name}
	}
	cmdline := ""
	for _, arg := range argv {
		cmdline += arg + "\x00"
	}
	write(t, filepath.Join(dir, "cmdline"), cmdline)
	fdDir := filepath.Join(dir, "fd")
	if err := os.MkdirAll(fdDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "fdinfo"), 0o755); err != nil {
		t.Fatal(err)
	}
	if spec.noDRM {
		return
	}
	if err := os.Symlink(testNode, filepath.Join(fdDir, "9")); err != nil {
		t.Fatal(err)
	}
	body := "pos:\t0\nflags:\t02100002\ndrm-driver:\tamdgpu\n" +
		"drm-client-id:\t" + spec.client + "\ndrm-pdev:\t" + spec.pdev + "\n" +
		"drm-total-vram:\t0 B\ndrm-resident-vram:\t0 B\n"
	for _, line := range spec.fdinfo {
		body += line + "\n"
	}
	write(t, filepath.Join(dir, "fdinfo", "9"), body)
}

// newSampler builds a sampler rooted at the synthetic trees.
func newSampler(drmRoot, pciRoot, procRoot string) *Sampler {
	return New(SamplerOptions{
		IntervalS:        0.05,
		DrmRoot:          drmRoot,
		PciRoot:          pciRoot,
		ProcRoot:         procRoot,
		CollectProcesses: true,
	})
}

// -- tests --------------------------------------------------------------

func TestOptionsValidated(t *testing.T) {
	opts := SamplerOptions{IntervalS: 0.001, HistoryLength: 0}.validated()
	if opts.IntervalS != MinIntervalS {
		t.Fatalf("interval = %v, want floor %v", opts.IntervalS, MinIntervalS)
	}
	if opts.HistoryLength != 1 {
		t.Fatalf("history length = %d, want 1", opts.HistoryLength)
	}
	if opts.DrmRoot != DefaultDrmRoot || opts.ProcRoot != DefaultProcRoot {
		t.Fatalf("roots = %q/%q, want defaults", opts.DrmRoot, opts.ProcRoot)
	}
}

func TestClampInterval(t *testing.T) {
	if got := clampInterval(-1); got != MinIntervalS {
		t.Fatalf("clamp(-1) = %v, want floor", got)
	}
	if got := clampInterval(1e9); got != MaxIntervalS {
		t.Fatalf("clamp(1e9) = %v, want ceiling", got)
	}
	if got := clampInterval(2.5); got != 2.5 {
		t.Fatalf("clamp(2.5) = %v, want passthrough", got)
	}
}

func TestEmptyTreeProducesAnEmptySnapshot(t *testing.T) {
	drmRoot, pciRoot := drmFixture(t)
	procRoot := procFixture(t)
	s := newSampler(drmRoot, pciRoot, procRoot)

	if got := s.Discover(); len(got) != 0 {
		t.Fatalf("discover = %v, want empty", got)
	}
	snap, err := s.SampleOnce()
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Devices) != 0 {
		t.Fatalf("devices = %v, want none", snap.Devices)
	}
	if len(snap.Processes) != 0 {
		t.Fatalf("processes = %v, want none", snap.Processes)
	}
	if snap.Sequence != 1 {
		t.Fatalf("sequence = %d, want 1", snap.Sequence)
	}
}

func TestSnapshotCombinesDevicesAndProcesses(t *testing.T) {
	drmRoot, pciRoot := drmFixture(t)
	procRoot := procFixture(t)
	makeGpu(t, drmRoot, pciRoot, gpuSpec{metrics: map[string]int{"temperature_edge": 49}})
	makeProc(t, procRoot, 100, procSpec{})
	s := newSampler(drmRoot, pciRoot, procRoot)
	s.Discover()

	snap, err := s.SampleOnce()
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Devices) != 1 {
		t.Fatalf("devices = %d, want 1", len(snap.Devices))
	}
	if len(snap.Processes) != 1 {
		t.Fatalf("processes = %d, want 1", len(snap.Processes))
	}
	if snap.TotalProcessCount != 1 {
		t.Fatalf("total = %d, want 1", snap.TotalProcessCount)
	}
	if snap.Stats.Ticks != 1 {
		t.Fatalf("ticks = %d, want 1", snap.Stats.Ticks)
	}
	if snap.IsPartialProcessView() {
		t.Fatal("a fully visible process must not read as partial")
	}
}

func TestPartialProcessVisibilityIsReported(t *testing.T) {
	drmRoot, pciRoot := drmFixture(t)
	procRoot := procFixture(t)
	makeGpu(t, drmRoot, pciRoot, gpuSpec{})
	makeProc(t, procRoot, 100, procSpec{})
	// A second process whose fd directory cannot be inspected (another user's,
	// without root): total sees it, visible does not.
	hidden := filepath.Join(procRoot, "200")
	if err := os.MkdirAll(hidden, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(procRoot, "999", "fd"), filepath.Join(hidden, "fd")); err != nil {
		t.Fatal(err)
	}
	s := newSampler(drmRoot, pciRoot, procRoot)
	s.Discover()

	snap, err := s.SampleOnce()
	if err != nil {
		t.Fatal(err)
	}
	if snap.TotalProcessCount != 2 {
		t.Fatalf("total = %d, want 2", snap.TotalProcessCount)
	}
	if snap.VisibleProcessCount != 1 {
		t.Fatalf("visible = %d, want 1", snap.VisibleProcessCount)
	}
	if !snap.IsPartialProcessView() {
		t.Fatal("hidden processes must be reported, not silently dropped")
	}
}

func TestDeviceWithoutBinaryMetricsStillProducesASample(t *testing.T) {
	drmRoot, pciRoot := drmFixture(t)
	procRoot := procFixture(t)
	makeGpu(t, drmRoot, pciRoot, gpuSpec{noMetrics: true, extraSysfs: map[string]string{
		"pp_dpm_sclk":             "0: 500Mhz *\n1: 2400Mhz \n",
		"mem_info_vis_vram_used":  "1048576",
		"mem_info_vis_vram_total": "8589934592",
		"gpu_busy_percent":        "42",
	}})
	s := newSampler(drmRoot, pciRoot, procRoot)
	s.Discover()

	snap, err := s.SampleOnce()
	if err != nil {
		t.Fatal(err)
	}
	m := snap.Devices[0]
	if m.MetricsABI != nil {
		t.Fatalf("abi = %v, want nil (no binary table)", *m.MetricsABI)
	}
	if m.SCLK.Source != "dpm" {
		t.Fatalf("sclk source = %q, want dpm", m.SCLK.Source)
	}
	if m.SCLK.Current == nil || *m.SCLK.Current != 500 || m.SCLK.Maximum == nil || *m.SCLK.Maximum != 2400 {
		t.Fatalf("sclk = %+v, want 500/2400 from dpm", m.SCLK)
	}
	if m.GpuBusyPercent == nil || *m.GpuBusyPercent != 42 {
		t.Fatalf("gpu busy = %v, want 42", m.GpuBusyPercent)
	}
	if m.VRAM.Used == nil || *m.VRAM.Used != 1048576 {
		t.Fatalf("vram used = %v, want 1048576", m.VRAM.Used)
	}
	if pct := m.VRAM.Percent(); pct == nil || *pct < 0.0121 || *pct > 0.0123 {
		t.Fatalf("vram percent = %v, want ~0.0122 (1MiB/8GiB)", pct)
	}
	if snap.Source.UsingBinaryMetrics() {
		t.Fatal("source must not claim binary metrics without a table")
	}
}

func TestOneBrokenDeviceDoesNotHideTheOthers(t *testing.T) {
	drmRoot, pciRoot := drmFixture(t)
	procRoot := procFixture(t)
	makeGpu(t, drmRoot, pciRoot, gpuSpec{card: 0, bdf: "0000:0c:00.0", metrics: map[string]int{"temperature_edge": 49}})
	makeGpu(t, drmRoot, pciRoot, gpuSpec{card: 1, bdf: "0000:03:00.0", metrics: map[string]int{"temperature_edge": 40}})
	s := newSampler(drmRoot, pciRoot, procRoot)
	devices := s.Discover()
	if len(devices) != 2 {
		t.Fatalf("discovered %d devices, want 2", len(devices))
	}
	// Make one device's gpu_metrics unreadable (a directory where a file was).
	broken := filepath.Join(devices[0].DeviceDir, "gpu_metrics")
	if err := os.Remove(broken); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(broken, 0o755); err != nil {
		t.Fatal(err)
	}

	snap, err := s.SampleOnce()
	if err != nil {
		t.Fatal(err)
	}
	// Both devices still produce a sample; the broken one simply has no binary
	// data (the readers are total -- a directory reads as absent).
	if len(snap.Devices) != 2 {
		t.Fatalf("devices = %d, want 2 (both still sampled)", len(snap.Devices))
	}
	var withABI, withoutABI int
	for _, m := range snap.Devices {
		if m.MetricsABI == nil {
			withoutABI++
			if m.SCLK.Current != nil {
				t.Fatalf("broken device sclk = %v, want nil", *m.SCLK.Current)
			}
		} else {
			withABI++
			if len(m.Temperatures) == 0 || *m.Temperatures[0].Celsius != 49 {
				t.Fatalf("healthy device temps = %v, want edge=49", m.Temperatures)
			}
		}
	}
	if withABI != 1 || withoutABI != 1 {
		t.Fatalf("abi split = %d/%d, want 1 healthy + 1 broken", withABI, withoutABI)
	}
}

func TestSampleOnceIsRepeatable(t *testing.T) {
	drmRoot, pciRoot := drmFixture(t)
	procRoot := procFixture(t)
	makeGpu(t, drmRoot, pciRoot, gpuSpec{metrics: map[string]int{"temperature_edge": 49}})
	s := newSampler(drmRoot, pciRoot, procRoot)
	s.Discover()

	first, err := s.SampleOnce()
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.SampleOnce()
	if err != nil {
		t.Fatal(err)
	}
	if first.Sequence != 1 || second.Sequence != 2 {
		t.Fatalf("sequences = %d,%d, want 1,2", first.Sequence, second.Sequence)
	}
	if first.TimestampNs > second.TimestampNs {
		t.Fatalf("timestamps not monotonic: %d > %d", first.TimestampNs, second.TimestampNs)
	}
	if len(s.History()) != 2 {
		t.Fatalf("history = %d, want 2 (sample_once populates it)", len(s.History()))
	}
}

func TestSourceReportTracksTheBinaryABI(t *testing.T) {
	drmRoot, pciRoot := drmFixture(t)
	procRoot := procFixture(t)
	makeGpu(t, drmRoot, pciRoot, gpuSpec{metrics: map[string]int{"temperature_edge": 49}})
	s := newSampler(drmRoot, pciRoot, procRoot)
	s.Discover()

	snap, err := s.SampleOnce()
	if err != nil {
		t.Fatal(err)
	}
	if snap.Source.MetricsABI != "v1.3" {
		t.Fatalf("abi = %q, want v1.3", snap.Source.MetricsABI)
	}
	if !snap.Source.UsingBinaryMetrics() {
		t.Fatal("source must report binary metrics")
	}
}

func TestLatestReturnsTheNewestQueuedSnapshot(t *testing.T) {
	drmRoot, pciRoot := drmFixture(t)
	procRoot := procFixture(t)
	s := newSampler(drmRoot, pciRoot, procRoot)
	s.Discover()
	for i := 0; i < 3; i++ {
		if _, err := s.SampleOnce(); err != nil {
			t.Fatal(err)
		}
	}

	newest := s.Latest()
	if newest == nil {
		t.Fatal("latest = nil, want a snapshot")
	}
	if newest.Sequence != 3 {
		t.Fatalf("latest sequence = %d, want 3 (drained)", newest.Sequence)
	}
	if s.Latest() != nil {
		t.Fatal("latest must be nil after draining")
	}
}

func TestSampleOnceRefusesWhileRunning(t *testing.T) {
	drmRoot, pciRoot := drmFixture(t)
	procRoot := procFixture(t)
	makeGpu(t, drmRoot, pciRoot, gpuSpec{metrics: map[string]int{"temperature_edge": 49}})
	s := newSampler(drmRoot, pciRoot, procRoot)
	s.Start()
	defer s.Stop()
	if snap, err := s.SampleOnce(); err == nil {
		t.Fatalf("sample_once must refuse to race the tick thread, got seq %d", snap.Sequence)
	}
}

// awaitSnapshot polls for one snapshot, keeping it: Latest empties the channel
// when it returns.
func awaitSnapshot(s *Sampler, timeout time.Duration) *model.GpuSnapshot {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if snap := s.Latest(); snap != nil {
			return snap
		}
		time.Sleep(5 * time.Millisecond)
	}
	return nil
}

func TestThreadPublishesSnapshots(t *testing.T) {
	drmRoot, pciRoot := drmFixture(t)
	procRoot := procFixture(t)
	makeGpu(t, drmRoot, pciRoot, gpuSpec{metrics: map[string]int{"temperature_edge": 49}})
	s := newSampler(drmRoot, pciRoot, procRoot)
	s.Start()

	snap := s.Wait(5 * time.Second)
	s.Stop()

	if snap == nil {
		t.Fatal("no snapshot published")
	}
	if len(snap.Devices) != 1 {
		t.Fatalf("devices = %d, want 1", len(snap.Devices))
	}
	if len(s.History()) == 0 {
		t.Fatal("history must be populated by the thread")
	}
}

func TestSetIntervalChangesTheRateTheThreadSamplesAt(t *testing.T) {
	drmRoot, pciRoot := drmFixture(t)
	procRoot := procFixture(t)
	makeGpu(t, drmRoot, pciRoot, gpuSpec{metrics: map[string]int{"temperature_edge": 49}})
	s := New(SamplerOptions{
		IntervalS: 30, DrmRoot: drmRoot, PciRoot: pciRoot, ProcRoot: procRoot,
		CollectProcesses: false,
	})
	s.Start()
	defer s.Stop()

	if s.Wait(5*time.Second) == nil {
		t.Fatal("the first tick did not happen")
	}
	if got := s.SetInterval(0.05); got != 0.05 {
		t.Fatalf("set_interval returned %v, want 0.05", got)
	}

	deadline := time.Now().Add(5 * time.Second)
	ticks := 0
	for time.Now().Before(deadline) && ticks < 5 {
		if s.Wait(2*time.Second) != nil {
			ticks++
		}
	}
	if ticks < 5 {
		t.Fatalf("only %d ticks after set_interval(0.05); the loop kept its 30s period", ticks)
	}
}

func TestRequestTickWakesASleepingSampler(t *testing.T) {
	drmRoot, pciRoot := drmFixture(t)
	procRoot := procFixture(t)
	makeGpu(t, drmRoot, pciRoot, gpuSpec{metrics: map[string]int{"temperature_edge": 49}})
	s := New(SamplerOptions{
		IntervalS: 30, DrmRoot: drmRoot, PciRoot: pciRoot, ProcRoot: procRoot,
		CollectProcesses: false,
	})
	s.Start()
	defer s.Stop()

	first := awaitSnapshot(s, 5*time.Second)
	if first == nil {
		t.Fatal("the first tick did not happen")
	}
	s.RequestTick()
	second := awaitSnapshot(s, 5*time.Second)
	if second == nil {
		t.Fatal("request_tick did not produce a sample")
	}
	if second.Sequence <= first.Sequence {
		t.Fatalf("second seq %d not after first %d", second.Sequence, first.Sequence)
	}
}
