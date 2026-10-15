package readers

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/gputop/gputop-go/internal/model"
)

// fullLine is a complete RDNA2 dump line exactly as dump.c formats it, with
// every optional field present.
const fullLine = "1759248000.123456: bus 0c, " +
	"gpu 45.31%, ee 12.50%, vgt 3.25%, ta 1.00%, tc 0.50%, sx 7.75%, sh 8.25%, " +
	"spi 2.50%, smx 1.00%, sc 1.25%, pa 1.00%, db 9.99%, cr 0.10%, cb 10.10%, " +
	"uvd 0.00%, vce0 0.00%, " +
	"vram 13.13% 4096.00mb, gtt 14.14% 512.00mb, " +
	"mclk 50.00% 1.000ghz, sclk 25.00% 2.104ghz"

// legacyLine is an R600-era line: no tc, smx, cr, uvd, vce0, no memory and no
// clocks. The conditional fields are the whole reason parsing is by name.
const legacyLine = "100.500000: bus 01, gpu 10.00%, ee 0.00%, vgt 0.00%, ta 0.00%, " +
	"sx 0.00%, sh 0.00%, spi 0.00%, sc 0.00%, pa 0.00%, db 0.00%, cb 0.00%"

// writeStandInScript writes an executable shell script standing in for
// radeontop and returns its path.
func writeStandInScript(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "radeontop-stand-in")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body), 0o755); err != nil {
		t.Fatalf("write stand-in: %v", err)
	}
	return path
}

// makeStandInPool returns an enabled pool whose single source runs body.
func makeStandInPool(t *testing.T, body string, mutate ...func(*RadeontopOptions)) *RadeontopPool {
	t.Helper()
	opts := RadeontopOptions{Enabled: true, Binary: writeStandInScript(t, body)}
	for _, m := range mutate {
		m(&opts)
	}
	pool := NewRadeontopPool(opts)
	pool.Configure([]model.AmdgpuDevice{{Index: 0, Card: "card0", BDF: "0000:0c:00.0"}})
	return pool
}

// -- parsing ---------------------------------------------------------------

func TestParseDumpLineReadsEveryField(t *testing.T) {
	dump := ParseDumpLine(fullLine)
	if dump == nil {
		t.Fatal("full line did not parse")
	}
	if dump.Seconds != 1759248000 || dump.Microseconds != 123456 {
		t.Errorf("timestamp = %d.%d", dump.Seconds, dump.Microseconds)
	}
	if dump.Bus != "0c" {
		t.Errorf("bus = %q", dump.Bus)
	}
	if got := dump.Blocks["gpu"]; got != 45.31 {
		t.Errorf("gpu = %v", got)
	}
	if got := dump.Blocks["cb"]; got != 10.10 {
		t.Errorf("cb = %v", got)
	}
	if got := dump.Clocks["sclk"]; got != [2]float64{25.0, 2104.0} {
		t.Errorf("sclk = %v", got)
	}
	if got := dump.Clocks["mclk"]; got != [2]float64{50.0, 1000.0} {
		t.Errorf("mclk = %v", got)
	}
	if got := dump.Memory["vram"]; got != [2]float64{13.13, 4096.0} {
		t.Errorf("vram = %v", got)
	}
}

func TestParseDumpLineOrdersBlocksByPipelineStage(t *testing.T) {
	blocks := ParseDumpLine(fullLine).ToBlocks(nil)
	got := make([]string, 0, len(blocks.Blocks))
	for _, b := range blocks.Blocks {
		got = append(got, b.Key)
	}
	if strings.Join(got, ",") != strings.Join(model.BlockOrder, ",") {
		t.Errorf("block order = %v, want %v", got, model.BlockOrder)
	}
}

func TestParseDumpLineLegacyLineOmitsAbsentBlocks(t *testing.T) {
	dump := ParseDumpLine(legacyLine)
	if dump == nil {
		t.Fatal("legacy line did not parse")
	}
	want := []string{"gpu", "ee", "vgt", "ta", "sx", "sh", "spi", "sc", "pa", "db", "cb"}
	if len(dump.Blocks) != len(want) {
		t.Fatalf("blocks = %v, want %d of them", dump.Blocks, len(want))
	}
	for _, key := range want {
		if _, ok := dump.Blocks[key]; !ok {
			t.Errorf("missing block %q", key)
		}
	}
	if len(dump.Clocks) != 0 || len(dump.Memory) != 0 {
		t.Errorf("legacy line carried clocks %v / memory %v", dump.Clocks, dump.Memory)
	}
}

func TestParseDumpLineMissingOptionalFieldDoesNotShift(t *testing.T) {
	// Without tc the value after it must stay with its own key: a positional
	// parser would read vgt's value as ta's on every modern card.
	withTC := ParseDumpLine(fullLine).Blocks
	without := ParseDumpLine(strings.Replace(fullLine, "tc 0.50%, ", "", 1)).Blocks
	if _, ok := without["tc"]; ok {
		t.Error("tc survived its own removal")
	}
	for _, key := range []string{"ta", "vgt", "sx"} {
		if without[key] != withTC[key] {
			t.Errorf("%s = %v, want %v", key, without[key], withTC[key])
		}
	}
}

func TestParseDumpLineAcceptsTrailingNewline(t *testing.T) {
	if ParseDumpLine(fullLine+"\n") == nil {
		t.Error("trailing newline rejected")
	}
}

func TestParseDumpLineRejectsBannersAndDiagnostics(t *testing.T) {
	for _, line := range []string{
		"",
		"Dumping to -, until termination.\n",
		"Failed to find DRM devices: error 2 (No such file or directory)\n",
		"radeontop 1.4.3, running on radeon bus 0c, 120 samples/sec\n",
		"0.5: bus zz, gpu 1.00%\n",
	} {
		if ParseDumpLine(line) != nil {
			t.Errorf("%q parsed as data", line)
		}
	}
}

func TestParseDumpLineIgnoresUnknownKeys(t *testing.T) {
	dump := ParseDumpLine("5.0: bus 01, gpu 10.00%, brandnew 42.00%, cb 1.00%")
	if dump == nil {
		t.Fatal("line did not parse")
	}
	if _, ok := dump.Blocks["brandnew"]; ok {
		t.Error("an unknown block key was guessed into the table")
	}
	if dump.Blocks["cb"] != 1.0 {
		t.Errorf("cb = %v", dump.Blocks["cb"])
	}
}

func TestToBlocksClocksCarryPercentAndFrequency(t *testing.T) {
	blocks := ParseDumpLine(fullLine).ToBlocks(nil)
	got := map[string]model.ClockReading{}
	for _, c := range blocks.Clocks {
		got[c.Key] = c
	}
	sclk := got["sclk"]
	if sclk.Label != "Shader Clock" || sclk.Percent == nil || *sclk.Percent != 25.0 ||
		sclk.Mhz == nil || *sclk.Mhz != 2104 {
		t.Errorf("sclk = %+v", sclk)
	}
	mclk := got["mclk"]
	if mclk.Label != "Memory Clock" || mclk.Mhz == nil || *mclk.Mhz != 1000 {
		t.Errorf("mclk = %+v", mclk)
	}
}

func TestToBlocksEmptySampleHasNoBusiest(t *testing.T) {
	empty := model.GpuBlocks{}
	if empty.Busiest() != nil || !empty.IsEmpty() {
		t.Error("an empty sample reported a busiest block")
	}
	busiest := ParseDumpLine(fullLine).ToBlocks(nil).Busiest()
	if busiest == nil || busiest.Key != "gpu" || busiest.Percent != 45.31 {
		t.Errorf("busiest = %+v", busiest)
	}
}

// -- command line ----------------------------------------------------------

func TestBuildArgvRequestsDumpMode(t *testing.T) {
	argv := BuildArgv(RadeontopOptions{}, "", "")
	if !strings.HasPrefix(strings.Join(argv, " "), "radeontop -d - -i 1") {
		t.Errorf("argv = %v", argv)
	}
	if argv[indexOf(argv, "-l")+1] != "0" {
		t.Errorf("line limit = %v, want 0 (until terminated)", argv)
	}
}

func TestBuildArgvPrefersNodeOverBus(t *testing.T) {
	argv := BuildArgv(RadeontopOptions{}, "0c", "/dev/dri/card1")
	if !contains(argv, "-p") || !contains(argv, "/dev/dri/card1") {
		t.Errorf("argv = %v, want -p /dev/dri/card1", argv)
	}
	if contains(argv, "-b") {
		t.Errorf("argv = %v, must not carry -b as well", argv)
	}
}

func TestBuildArgvUsesBusWithoutNode(t *testing.T) {
	argv := BuildArgv(RadeontopOptions{}, "0c", "")
	if got := argv[indexOf(argv, "-b")+1]; got != "0c" {
		t.Errorf("bus = %q, want 0c", got)
	}
}

func TestBuildArgvPassesTicksRate(t *testing.T) {
	argv := BuildArgv(RadeontopOptions{Ticks: 50}, "", "")
	if got := argv[indexOf(argv, "-t")+1]; got != "50" {
		t.Errorf("ticks = %q, want 50", got)
	}
}

func TestBusFromBDF(t *testing.T) {
	for bdf, want := range map[string]string{
		"0000:0c:00.0":  "0c",
		"0000:01:00.0":  "01",
		"10000:ff:1f.7": "ff",
	} {
		got, ok := BusFromBDF(bdf)
		if !ok || got != want {
			t.Errorf("BusFromBDF(%q) = %q, %v; want %q, true", bdf, got, ok, want)
		}
	}
}

func TestBusFromBDFMalformed(t *testing.T) {
	for _, bdf := range []string{"", "nonsense", "0000:zz:00.0", "0c:00.0"} {
		if _, ok := BusFromBDF(bdf); ok {
			t.Errorf("BusFromBDF(%q) accepted a malformed address", bdf)
		}
	}
}

func TestOptionsValidatedClampsInterval(t *testing.T) {
	// radeontop parses -i with atoi and floors it at 1; honouring 500 ms here
	// would produce a panel four times slower than the user asked for.
	if got := (RadeontopOptions{IntervalS: 0}).Validated().IntervalS; got != MinDumpIntervalS {
		t.Errorf("interval 0 -> %d", got)
	}
	if got := (RadeontopOptions{IntervalS: -5}).Validated().IntervalS; got != MinDumpIntervalS {
		t.Errorf("interval -5 -> %d", got)
	}
	if got := (RadeontopOptions{IntervalS: 3}).Validated().IntervalS; got != 3 {
		t.Errorf("interval 3 -> %d", got)
	}
	if got := (RadeontopOptions{Ticks: 0}).Validated().Ticks; got != 1 {
		t.Errorf("ticks 0 -> %d", got)
	}
}

// -- binary resolution -----------------------------------------------------

func TestResolveBinaryFindsExecutablePath(t *testing.T) {
	target := writeStandInScript(t, "exit 0\n")
	got, ok := ResolveBinary(target)
	if !ok || got != target {
		t.Errorf("ResolveBinary(%q) = %q, %v", target, got, ok)
	}
}

func TestResolveBinaryMissing(t *testing.T) {
	if _, ok := ResolveBinary("gputop-no-such-binary-xyz"); ok {
		t.Error("a missing binary resolved")
	}
}

func TestResolveBinaryRejectsNonExecutable(t *testing.T) {
	plain := filepath.Join(t.TempDir(), "radeontop")
	if err := os.WriteFile(plain, []byte("not executable"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, ok := ResolveBinary(plain); ok {
		t.Error("a non-executable file resolved as a binary")
	}
}

// -- failure classification ------------------------------------------------

func TestClassifyFailureMapsKnownMessages(t *testing.T) {
	cases := map[string]model.BlocksStatus{
		"Cannot access GPU registers, are you root?": model.BlocksNotPermitted,
		"Failed to open DRM node, no VRAM support.":  model.BlocksNotPermitted,
		"open /dev/dri/card0: Permission denied":     model.BlocksNotPermitted,
		"Failed to find DRM devices: error 2":        model.BlocksUnsupported,
		"Unsupported bus type 5":                     model.BlocksUnsupported,
		"Failed to open /dev/mem":                    model.BlocksUnsupported,
	}
	for line, want := range cases {
		got, ok := ClassifyFailure([]string{line})
		if !ok || got != want {
			t.Errorf("ClassifyFailure(%q) = %q, %v; want %q", line, got, ok, want)
		}
	}
}

func TestClassifyFailureRecognisesStdoutComplaint(t *testing.T) {
	// radeontop splits its complaints across both streams; reading stderr
	// alone classifies a privilege problem as an unsupported card.
	for _, line := range []string{
		"Failed to open DRM node, no VRAM support.\n",
		"Cannot access GPU registers, are you root?\n",
	} {
		if got, ok := ClassifyFailure([]string{line}); !ok || got != model.BlocksNotPermitted {
			t.Errorf("ClassifyFailure(%q) = %q, %v", line, got, ok)
		}
	}
}

func TestClassifyFailureIgnoresDataAndBanners(t *testing.T) {
	if _, ok := ClassifyFailure([]string{fullLine}); ok {
		t.Error("a dump line was classified as a complaint")
	}
	if _, ok := ClassifyFailure([]string{"Dumping to -, until termination."}); ok {
		t.Error("the success banner was classified as a failure")
	}
}

func TestClassifyFailurePermissionWinsOverUnrelatedErrors(t *testing.T) {
	// radeontop reports every problem it has; the actionable one comes last.
	lines := []string{
		"Failed to find DRM devices: error 2 (No such file or directory)",
		"Failed to open DRM node, no VRAM support.",
		"Cannot access GPU registers, are you root?",
	}
	if got, ok := ClassifyFailure(lines); !ok || got != model.BlocksNotPermitted {
		t.Errorf("ClassifyFailure = %q, %v; want not permitted", got, ok)
	}
}

func TestClassifyFailureSilentIsUnclassified(t *testing.T) {
	if _, ok := ClassifyFailure(nil); ok {
		t.Error("empty output was classified")
	}
	if _, ok := ClassifyFailure([]string{"something entirely unexpected"}); ok {
		t.Error("an unknown message was classified")
	}
}

// -- the supervised process ------------------------------------------------

func TestPoolParsesDumpLineFromChild(t *testing.T) {
	pool := makeStandInPool(t, `
echo "Dumping to -, until termination." >&2
while true; do
  echo '100.0: bus 0c, gpu 42.00%, cb 7.00%, sclk 25.00% 2.100ghz'
  sleep 0.05
done
`)
	pool.Start()
	defer pool.Stop()
	if !WaitForFirstSample(pool, 10*time.Second) {
		t.Fatal("the child never produced a sample")
	}
	if got := pool.Status(); got != model.BlocksOK {
		t.Errorf("status = %q, want ok", got)
	}
	blocks := pool.BlocksFor(0)
	if blocks == nil {
		t.Fatal("no sample for device 0")
	}
	if busiest := blocks.Busiest(); busiest == nil || busiest.Key != "gpu" {
		t.Errorf("busiest = %+v", busiest)
	}
	if len(blocks.Clocks) == 0 || blocks.Clocks[0].Mhz == nil || *blocks.Clocks[0].Mhz != 2100 {
		t.Errorf("clocks = %+v", blocks.Clocks)
	}
}

func TestPoolChildThatExitsAtOnceIsFailed(t *testing.T) {
	pool := makeStandInPool(t, "exit 0\n")
	pool.Start()
	defer pool.Stop()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) && pool.Status() == model.BlocksOK {
		pool.Poll()
		time.Sleep(20 * time.Millisecond)
	}
	if got := pool.Status(); got != model.BlocksFailed {
		t.Errorf("status = %q, want failed", got)
	}
	if pool.BlocksFor(0) != nil {
		t.Error("a failed child reported a sample")
	}
}

func TestPoolPermissionFailureIsNamed(t *testing.T) {
	pool := makeStandInPool(t, `
echo 'Cannot access GPU registers, are you root?' >&2
exit 0
`)
	pool.Start()
	defer pool.Stop()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) && pool.Status() == model.BlocksOK {
		pool.Poll()
		time.Sleep(20 * time.Millisecond)
	}
	if got := pool.Status(); got != model.BlocksNotPermitted {
		t.Errorf("status = %q, want not permitted", got)
	}
	if pool.BlocksFor(0) != nil {
		t.Error("a refused child reported a sample")
	}
}

func TestPoolMissingBinaryNeverStarts(t *testing.T) {
	pool := NewRadeontopPool(RadeontopOptions{Enabled: true, Binary: "gputop-no-such-binary-xyz"})
	pool.Configure([]model.AmdgpuDevice{{Index: 0, Card: "card0", BDF: "0000:0c:00.0"}})
	pool.Start()
	defer pool.Stop()
	if got := pool.Status(); got != model.BlocksMissing {
		t.Errorf("status = %q, want missing", got)
	}
	if pool.BlocksFor(0) != nil {
		t.Error("a missing binary reported a sample")
	}
}

func TestPoolDisabledStartsNothing(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "ran")
	binary := writeStandInScript(t, "touch "+marker+"\nsleep 5\n")
	pool := NewRadeontopPool(RadeontopOptions{Enabled: false, Binary: binary})
	pool.Configure([]model.AmdgpuDevice{{Index: 0, Card: "card0", BDF: "0000:0c:00.0"}})
	pool.Start()
	time.Sleep(200 * time.Millisecond)
	pool.Stop()
	if got := pool.Status(); got != model.BlocksDisabled {
		t.Errorf("status = %q, want disabled", got)
	}
	if _, err := os.Stat(marker); err == nil {
		t.Error("a disabled pool started a child")
	}
}

func TestPoolStopClosesPipes(t *testing.T) {
	pool := makeStandInPool(t, "while true; do sleep 0.05; done\n")
	pool.Start()
	pool.Stop()
	if pool.sources[0].child != nil {
		t.Error("stop left the child handle set")
	}
}

func TestPoolStopLeavesNoChildBehind(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "pid")
	pool := makeStandInPool(t, "echo $$ > "+pidFile+"\nwhile true; do sleep 0.05; done\n")
	pool.Start()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(pidFile); err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	data, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatalf("child never wrote its pid: %v", err)
	}
	var pid int
	if _, err := fmt.Sscan(strings.TrimSpace(string(data)), &pid); err != nil {
		t.Fatalf("bad pid file %q: %v", data, err)
	}
	pool.Stop()
	deadline = time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if err := syscall.Kill(pid, 0); errors.Is(err, syscall.ESRCH) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("child %d survived Stop", pid)
}

func TestPoolTwoDevicesGetOneChildEach(t *testing.T) {
	pool := NewRadeontopPool(RadeontopOptions{
		Enabled: true,
		Binary:  writeStandInScript(t, "sleep 5\n"),
	})
	pool.Configure([]model.AmdgpuDevice{
		{Index: 0, Card: "card0", BDF: "0000:0c:00.0"},
		{Index: 1, Card: "card1", BDF: "0000:01:00.0"},
	})
	pool.Start()
	defer pool.Stop()
	if len(pool.sources) != 2 {
		t.Errorf("sources = %d, want 2", len(pool.sources))
	}
	if got := pool.Status(); got != model.BlocksOK {
		t.Errorf("status = %q, want ok", got)
	}
}

func TestPoolDeviceWithNoSourceHasNoSample(t *testing.T) {
	pool := makeStandInPool(t, "sleep 5\n")
	defer pool.Stop()
	if pool.BlocksFor(7) != nil {
		t.Error("an unknown device index reported a sample")
	}
}

func TestPoolConfigureStopsPreviousChildren(t *testing.T) {
	pool := makeStandInPool(t, "sleep 5\n")
	pool.Start()
	pool.Configure([]model.AmdgpuDevice{{Index: 0, Card: "card0", BDF: "0000:0c:00.0"}})
	defer pool.Stop()
	if len(pool.sources) != 1 {
		t.Errorf("sources = %d, want 1", len(pool.sources))
	}
}

func TestPoolFailureNeverPanics(t *testing.T) {
	pool := NewRadeontopPool(RadeontopOptions{
		Enabled: true,
		Binary:  filepath.Join(t.TempDir(), "nope"),
	})
	pool.Configure([]model.AmdgpuDevice{{Index: 0, Card: "card0", BDF: "0000:0c:00.0"}})
	pool.Start()
	if got := pool.Status(); got != model.BlocksMissing {
		t.Errorf("status = %q, want missing", got)
	}
	pool.Stop()
}

// indexOf returns the position of want in argv, or -1.
func indexOf(argv []string, want string) int {
	for i, arg := range argv {
		if arg == want {
			return i
		}
	}
	return -1
}

// contains reports whether argv holds want.
func contains(argv []string, want string) bool { return indexOf(argv, want) >= 0 }
