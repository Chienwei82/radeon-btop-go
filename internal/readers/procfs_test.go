package readers

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/gputop/gputop-go/internal/model"
)

// Fixture constants: the shape a real amdgpu fdinfo reports.
const (
	testPdev    = "0000:0c:00.0"
	testPdevTwo = "0000:03:00.0"
	testNode    = "/dev/dri/renderD128"
	testNodeTwo = "/dev/dri/renderD129"
	testClient  = "42"
)

// testWindow is one second of wall clock, the delta window the maths is
// checked against.
const testWindow model.Nanoseconds = 1_000_000_000

// engineLine builds a drm-engine line in the kernel's exact format.
func engineLine(engine string, ns int64) string {
	return fmt.Sprintf("drm-engine-%s: %d ns", engine, ns)
}

// fdinfoBody writes the standard DRM header every fixture carries, then the
// caller's lines. Later lines win, so a test can override a default.
func fdinfoBody(client, pdev string, lines ...string) string {
	out := "pos:\t0\nflags:\t02100002\ndrm-driver:\tamdgpu\n" +
		"drm-client-id:\t" + client + "\ndrm-pdev:\t" + pdev + "\n" +
		"drm-total-vram:\t0 B\ndrm-resident-vram:\t0 B\n"
	for _, line := range lines {
		out += line + "\n"
	}
	return out
}

// procSpec is one fake process. A nil Cmdline defaults to the process name; an
// empty non-nil Cmdline is a kernel thread, which is how the kernel presents
// one. NoDRM writes no GPU descriptor at all, only ExtraFds.
type procSpec struct {
	Name     string
	Node     string
	Client   string
	Pdev     string
	PPid     int
	Cmdline  []string
	ExtraFds []string
	Fdinfo   []string
	NoDRM    bool
}

// makeProc writes a fake process holding one DRM descriptor.
func makeProc(t *testing.T, procRoot string, pid int, spec procSpec) {
	t.Helper()
	if spec.Name == "" {
		spec.Name = "testproc"
	}
	if spec.Node == "" {
		spec.Node = testNode
	}
	if spec.Client == "" {
		spec.Client = testClient
	}
	if spec.Pdev == "" {
		spec.Pdev = testPdev
	}
	if spec.PPid == 0 {
		spec.PPid = 1
	}
	dir := filepath.Join(procRoot, fmt.Sprint(pid))
	mustWrite(t, filepath.Join(dir, "comm"), spec.Name+"\n")
	mustWrite(t, filepath.Join(dir, "status"),
		fmt.Sprintf("Name:\t%s\nPPid:\t%d\n", spec.Name, spec.PPid))
	argv := spec.Cmdline
	if argv == nil {
		argv = []string{spec.Name}
	}
	cmdline := ""
	for _, arg := range argv {
		cmdline += arg + "\x00"
	}
	mustWrite(t, filepath.Join(dir, "cmdline"), cmdline)
	fdDir := filepath.Join(dir, "fd")
	if err := os.MkdirAll(fdDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "fdinfo"), 0o755); err != nil {
		t.Fatal(err)
	}
	if spec.NoDRM {
		for i, target := range spec.ExtraFds {
			link := filepath.Join(fdDir, fmt.Sprint(20+i))
			if err := os.Symlink(target, link); err != nil {
				t.Fatal(err)
			}
		}
		return
	}
	if err := os.Symlink(spec.Node, filepath.Join(fdDir, "9")); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(dir, "fdinfo", "9"),
		fdinfoBody(spec.Client, spec.Pdev, spec.Fdinfo...))
	for i, target := range spec.ExtraFds {
		link := filepath.Join(fdDir, fmt.Sprint(20+i))
		if err := os.Symlink(target, link); err != nil {
			t.Fatal(err)
		}
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// setFdinfo rewrites a fake process's fdinfo body, keeping the standard DRM
// header keys. Empty client/pdev keep the defaults, so a test that only bumps
// a counter keeps the same (pdev, client-id) key across both samples.
func setFdinfo(t *testing.T, procRoot string, pid int, client, pdev string, lines ...string) {
	t.Helper()
	if client == "" {
		client = testClient
	}
	if pdev == "" {
		pdev = testPdev
	}
	mustWrite(t, filepath.Join(procRoot, fmt.Sprint(pid), "fdinfo", "9"),
		fdinfoBody(client, pdev, lines...))
}

// setEngine sets one cumulative engine counter for a process.
func setEngine(t *testing.T, procRoot string, pid int, engine string, ns int64) {
	t.Helper()
	setFdinfo(t, procRoot, pid, "", "", engineLine(engine, ns))
}

// procFixture is an empty fake /proc.
func procFixture(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "proc")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestParseFdinfoRealShaped(t *testing.T) {
	text := "pos:\t0\nflags:\t02100002\ndrm-driver:\tamdgpu\ndrm-client-id:\t71\ndrm-pdev:\t0000:0c:00.0\n" +
		"drm-total-vram:\t4116 KiB\ndrm-resident-vram:\t4116 KiB\ndrm-shared-vram:\t0 B\n" +
		"drm-total-gtt:\t4108 KiB\ndrm-resident-gtt:\t4108 KiB\n" +
		"drm-engine-gfx: 173321 ns\ndrm-engine-compute: 4096 ns\n"
	rec := ParseFdinfo(text)
	if rec == nil || rec.Driver != "amdgpu" || rec.ClientID != 71 || rec.Pdev != "0000:0c:00.0" {
		t.Fatalf("record = %+v", rec)
	}
	if rec.Engines["gfx"] != 173321 || rec.Engines["compute"] != 4096 {
		t.Fatalf("engines = %v", rec.Engines)
	}
	// The kernel writes binary IEC prefixes: 4116 KiB is 4214784 bytes.
	if rec.Memory["drm-resident-vram"] != 4116*1024 {
		t.Fatalf("resident vram = %d", rec.Memory["drm-resident-vram"])
	}
	if rec.Memory["drm-total-gtt"] != 4108*1024 {
		t.Fatalf("total gtt = %d", rec.Memory["drm-total-gtt"])
	}
}

func TestParseFdinfoActivePeriodOverridesCumulative(t *testing.T) {
	text := "drm-driver:\tamdgpu\ndrm-client-id:\t5\n" +
		"drm-engine-gfx:\t999999 ns\n" +
		"drm-engine-active-gfx:\t250\ndrm-engine-period-gfx:\t1000\n"
	rec := ParseFdinfo(text)
	if rec == nil || rec.Engines["gfx"] != 250 {
		t.Fatalf("active/period must win: %+v", rec)
	}
}

func TestParseFdinfoRejects(t *testing.T) {
	if ParseFdinfo("drm-driver:\ti915\ndrm-client-id:\t1\n") != nil {
		t.Fatal("non-amdgpu must be nil")
	}
	if ParseFdinfo("pos:\t0\nflags:\t02100002\n") != nil {
		t.Fatal("non-DRM must be nil")
	}
	if ParseFdinfo("drm-driver:\tamdgpu\ndrm-client-id:\tboom\n") != nil {
		t.Fatal("non-numeric client id must be nil")
	}
}

func TestNormaliseEngine(t *testing.T) {
	cases := map[string]string{
		"gfx": "gfx", "compute": "compute",
		"sdma0": "dma", "sdma1": "dma", "sdma": "dma", "dma": "dma",
		"vangogh": "enc", "vcnenc": "enc", "venc": "enc", "encode": "enc",
		"jpeg": "dec", "vdec": "dec", "vdpdec": "dec", "uvd": "dec",
		"video": "other", "vcn": "other", "vcn0": "other",
		"something-new": "other",
	}
	for raw, want := range cases {
		if got := NormaliseEngine(raw); got != want {
			t.Fatalf("%s = %q, want %q", raw, got, want)
		}
	}
}

func TestCollectionFindsAClient(t *testing.T) {
	root := procFixture(t)
	makeProc(t, root, 100, procSpec{Fdinfo: []string{"drm-resident-vram:\t8192 KiB"}})

	processes, visible, total := NewProcessCollector(root).Collect(0)

	if total != 1 || visible != 1 || len(processes) != 1 {
		t.Fatalf("visible/total/rows = %d/%d/%d", visible, total, len(processes))
	}
	p := processes[0]
	if p.PID != 100 || p.Name != "testproc" {
		t.Fatalf("row = %+v", p)
	}
	if p.VramUsed == nil || *p.VramUsed != 8192*1024 {
		t.Fatalf("vram = %v", p.VramUsed)
	}
}

func TestNonDrmDescriptorsAreIgnored(t *testing.T) {
	root := procFixture(t)
	makeProc(t, root, 100, procSpec{
		NoDRM:    true,
		ExtraFds: []string{"/dev/null", "/etc/passwd", "/tmp/sock"},
	})

	processes, visible, total := NewProcessCollector(root).Collect(0)

	// Inspectable, so it counts as visible even with no GPU descriptor:
	// visible is about /proc permissions, not about holding a DRM fd.
	if len(processes) != 0 || total != 1 || visible != 1 {
		t.Fatalf("rows/visible/total = %d/%d/%d", len(processes), visible, total)
	}
}

func TestDuplicateClientIDsAreMergedNotSummed(t *testing.T) {
	root := procFixture(t)
	for _, pid := range []int{100, 200} {
		makeProc(t, root, pid, procSpec{
			Fdinfo: []string{"drm-resident-vram:\t4096 KiB", engineLine("gfx", 10_000)},
		})
	}

	processes, _, _ := NewProcessCollector(root).Collect(0)

	if len(processes) != 1 {
		t.Fatalf("rows = %d, want one merged row", len(processes))
	}
	// Maximum, not the sum of 4096 + 4096.
	if processes[0].VramUsed == nil || *processes[0].VramUsed != 4096*1024 {
		t.Fatalf("vram = %v", processes[0].VramUsed)
	}
	// The lowest pid is reported so the row is stable between scans.
	if processes[0].PID != 100 {
		t.Fatalf("pid = %d, want 100", processes[0].PID)
	}
}

func TestSameClientIDOnDifferentGPUsStaysSeparate(t *testing.T) {
	root := procFixture(t)
	makeProc(t, root, 100, procSpec{Client: "7"})
	makeProc(t, root, 200, procSpec{Client: "7", Node: testNodeTwo, Pdev: testPdevTwo})

	processes, _, _ := NewProcessCollector(root).Collect(0)

	if len(processes) != 2 {
		t.Fatalf("rows = %d, want 2", len(processes))
	}
	seen := map[string]bool{}
	for _, p := range processes {
		seen[p.BDF] = true
	}
	if !seen[testPdev] || !seen[testPdevTwo] {
		t.Fatalf("bdfs = %v", seen)
	}
}

func TestEnginePercentIsNilOnFirstSample(t *testing.T) {
	root := procFixture(t)
	makeProc(t, root, 100, procSpec{Fdinfo: []string{engineLine("gfx", 500_000_000)}})

	processes, _, _ := NewProcessCollector(root).Collect(0)

	usage := processes[0].Engines[0]
	if usage.TotalNs != 500_000_000 || usage.DeltaNs != 0 || usage.Percent() != nil {
		t.Fatalf("usage = %+v", usage)
	}
	if processes[0].EnginePercent() != 0 {
		t.Fatalf("engine percent = %v, want 0 for an unmeasured client", processes[0].EnginePercent())
	}
}

func TestEngineDeltaComputesPercent(t *testing.T) {
	root := procFixture(t)
	collector := NewProcessCollector(root)
	makeProc(t, root, 100, procSpec{Fdinfo: []string{engineLine("gfx", 100_000_000)}})
	collector.Collect(0)
	setEngine(t, root, 100, "gfx", 350_000_000)

	processes, _, _ := collector.Collect(testWindow)

	usage := processes[0].Engines[0]
	if usage.DeltaNs != 250_000_000 || usage.WindowNs != testWindow {
		t.Fatalf("usage = %+v", usage)
	}
	if processes[0].EnginePercent() != 25.0 {
		t.Fatalf("engine percent = %v, want 25", processes[0].EnginePercent())
	}
}

func TestEngineDeltaIsClampedToOneHundred(t *testing.T) {
	root := procFixture(t)
	collector := NewProcessCollector(root)
	makeProc(t, root, 100, procSpec{Fdinfo: []string{engineLine("gfx", 0)}})
	collector.Collect(0)
	setEngine(t, root, 100, "gfx", 5*testWindow)

	processes, _, _ := collector.Collect(testWindow)

	if processes[0].EnginePercent() != 100.0 {
		t.Fatalf("engine percent = %v, want 100", processes[0].EnginePercent())
	}
}

func TestCounterGoingBackwardsClampsToZero(t *testing.T) {
	root := procFixture(t)
	collector := NewProcessCollector(root)
	makeProc(t, root, 100, procSpec{Fdinfo: []string{engineLine("gfx", 400_000_000)}})
	collector.Collect(0)
	setEngine(t, root, 100, "gfx", 1_000_000)

	processes, _, _ := collector.Collect(testWindow)

	if processes[0].Engines[0].DeltaNs != 0 || processes[0].EnginePercent() != 0 {
		t.Fatalf("row = %+v", processes[0])
	}
}

func TestEmptyProcRootYieldsNothing(t *testing.T) {
	processes, visible, total := NewProcessCollector(
		filepath.Join(t.TempDir(), "does-not-exist")).Collect(0)
	if len(processes) != 0 || visible != 0 || total != 0 {
		t.Fatalf("rows/visible/total = %d/%d/%d", len(processes), visible, total)
	}
}

func TestNonNumericPidDirectoriesAreSkipped(t *testing.T) {
	root := procFixture(t)
	for _, name := range []string{"self", "sys", "irq"} {
		if err := os.MkdirAll(filepath.Join(root, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	makeProc(t, root, 100, procSpec{})

	_, visible, total := NewProcessCollector(root).Collect(0)

	if total != 1 || visible != 1 {
		t.Fatalf("visible/total = %d/%d, want 1/1", visible, total)
	}
}

func TestVanishedProcessIsSkipped(t *testing.T) {
	root := procFixture(t)
	makeProc(t, root, 100, procSpec{})
	ghost := filepath.Join(root, "200")
	if err := os.MkdirAll(ghost, 0o755); err != nil {
		t.Fatal(err)
	}
	// Dangling fd/fdinfo: exactly the shape of a process that exited between
	// the scan and the read.
	if err := os.Symlink(filepath.Join(root, "999", "fd"), filepath.Join(ghost, "fd")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "999", "fdinfo"), filepath.Join(ghost, "fdinfo")); err != nil {
		t.Fatal(err)
	}

	processes, visible, total := NewProcessCollector(root).Collect(0)

	if total != 2 || visible != 1 || len(processes) != 1 {
		t.Fatalf("visible/total/rows = %d/%d/%d", visible, total, len(processes))
	}
}

func TestResetForgetsTheDeltaBaseline(t *testing.T) {
	root := procFixture(t)
	collector := NewProcessCollector(root)
	makeProc(t, root, 100, procSpec{Fdinfo: []string{engineLine("gfx", 0)}})
	collector.Collect(0)
	setEngine(t, root, 100, "gfx", 10_000_000)
	if got, _, _ := collector.Collect(testWindow); got[0].EnginePercent() != 1.0 {
		t.Fatalf("engine percent = %v, want 1", got[0].EnginePercent())
	}

	collector.Reset()
	if got, _, _ := collector.Collect(2 * testWindow); got[0].EnginePercent() != 0 {
		t.Fatalf("after reset = %v, want unknown rather than a lie", got[0].EnginePercent())
	}
}

func TestProcessesAreSortedBusiestFirst(t *testing.T) {
	root := procFixture(t)
	collector := NewProcessCollector(root)
	for _, pid := range []int{100, 200, 300} {
		makeProc(t, root, pid, procSpec{Fdinfo: []string{engineLine("gfx", 0)}})
		setFdinfo(t, root, pid, fmt.Sprint(pid), testPdev, engineLine("gfx", 0))
	}
	collector.Collect(0)
	busy := map[int]int64{100: 100_000_000, 200: 700_000_000, 300: 400_000_000}
	for pid, ns := range busy {
		setFdinfo(t, root, pid, fmt.Sprint(pid), testPdev, engineLine("gfx", ns))
	}

	processes, _, _ := collector.Collect(testWindow)

	want := []int{200, 300, 100}
	for i, w := range want {
		if processes[i].PID != w {
			t.Fatalf("order = %v, want %v", pids(processes), want)
		}
	}
}

func TestCpuMemoryPoolIsCaptured(t *testing.T) {
	root := procFixture(t)
	makeProc(t, root, 100, procSpec{Fdinfo: []string{"drm-resident-cpu:\t16384 KiB"}})

	processes, _, _ := NewProcessCollector(root).Collect(0)

	if processes[0].CpuUsed == nil || *processes[0].CpuUsed != 16384*1024 {
		t.Fatalf("cpu used = %v", processes[0].CpuUsed)
	}
	if processes[0].MemoryUsed() != 16384*1024 {
		t.Fatalf("memory used = %d", processes[0].MemoryUsed())
	}
}

func TestProcessIdentityIsReadFromCommStatusCmdline(t *testing.T) {
	root := procFixture(t)
	makeProc(t, root, 100, procSpec{
		Name: "compositor", PPid: 42,
		Cmdline: []string{"/usr/bin/compositor", "--flag"},
	})

	processes, _, _ := NewProcessCollector(root).Collect(0)

	p := processes[0]
	if p.PPID == nil || *p.PPID != 42 {
		t.Fatalf("ppid = %v", p.PPID)
	}
	if got := p.Command(); got != "/usr/bin/compositor --flag" {
		t.Fatalf("command = %q", got)
	}
	if p.User == "" {
		t.Fatal("user must be resolved")
	}
	if p.IsKernelThread() {
		t.Fatal("a process with an argv is not a kernel thread")
	}
}

func TestKernelThreadHasNoCmdline(t *testing.T) {
	root := procFixture(t)
	makeProc(t, root, 100, procSpec{Name: "kworker", Cmdline: []string{}})

	processes, _, _ := NewProcessCollector(root).Collect(0)

	p := processes[0]
	if !p.IsKernelThread() {
		t.Fatal("empty argv must read as a kernel thread")
	}
	if got := p.Command(); got != "kworker" {
		t.Fatalf("command = %q, want the comm fallback", got)
	}
}

func pids(processes []model.GpuProcess) []int {
	out := make([]int, 0, len(processes))
	for _, p := range processes {
		out = append(out, p.PID)
	}
	return out
}

func TestMultiEngineUsageSumsThenClamps(t *testing.T) {
	root := procFixture(t)
	collector := NewProcessCollector(root)
	makeProc(t, root, 100, procSpec{
		Fdinfo: []string{engineLine("gfx", 0), engineLine("compute", 0)},
	})
	collector.Collect(0)
	setFdinfo(t, root, 100, "", "",
		engineLine("gfx", 400_000_000), engineLine("compute", 900_000_000))

	processes, _, _ := collector.Collect(testWindow)

	if processes[0].EnginePercent() != 100.0 {
		t.Fatalf("engine percent = %v, want 40+90 clamped to 100", processes[0].EnginePercent())
	}
}
