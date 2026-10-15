package check

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gputop/gputop-go/internal/model"
)

// RED: the check package does not exist yet.
func TestExplainFanOnIntegratedGPU(t *testing.T) {
	dir := t.TempDir()
	reason, detail := explain(dir, []string{"hwmon"}, [2]int{6, 8}, explainOpts{discreteOnly: true, integrated: true})
	if reason != ReasonUnsupportedGeneration {
		t.Fatalf("reason = %q, want %q", reason, ReasonUnsupportedGeneration)
	}
	if !strings.Contains(detail, "integrated GPU") {
		t.Fatalf("detail = %q, want the APU explanation", detail)
	}
}

func TestExplainOldKernelNamesBothVersions(t *testing.T) {
	dir := t.TempDir()
	reason, detail := explain(dir, []string{"gpu_busy_percent"}, [2]int{4, 9}, explainOpts{})
	if reason != ReasonNeedsNewerKernel {
		t.Fatalf("reason = %q, want %q", reason, ReasonNeedsNewerKernel)
	}
	if !strings.Contains(detail, "4.19") || !strings.Contains(detail, "4.9") {
		t.Fatalf("detail = %q, want both kernel versions", detail)
	}
}

func TestExplainPresentAttributeIsNotPublished(t *testing.T) {
	dir := t.TempDir()
	writeAttr(t, dir, "gpu_busy_percent", "")
	reason, detail := explain(dir, []string{"gpu_busy_percent"}, [2]int{6, 8}, explainOpts{})
	if reason != ReasonNotPublished {
		t.Fatalf("reason = %q, want %q", reason, ReasonNotPublished)
	}
	if !strings.Contains(detail, "gpu_busy_percent") {
		t.Fatalf("detail = %q, want the attribute named", detail)
	}
}

func TestExplainAbsentOnNewKernelIsNotPublished(t *testing.T) {
	dir := t.TempDir()
	reason, detail := explain(dir, []string{"mem_vram_total"}, [2]int{6, 8}, explainOpts{})
	if reason != ReasonNotPublished {
		t.Fatalf("reason = %q, want %q", reason, ReasonNotPublished)
	}
	if !strings.Contains(detail, "does not create") {
		t.Fatalf("detail = %q, want the driver explanation", detail)
	}
}

func TestExplainUnknownGPUMetricsABI(t *testing.T) {
	dir := t.TempDir()
	writeAttr(t, dir, "gpu_metrics", "binary blob the build cannot decode")
	reason, detail := explain(dir, []string{"gpu_metrics"}, [2]int{6, 8}, explainOpts{metricsABI: ""})
	if reason != ReasonUnknownABI {
		t.Fatalf("reason = %q, want %q", reason, ReasonUnknownABI)
	}
	if !strings.Contains(detail, "does not decode") {
		t.Fatalf("detail = %q, want the decoder explanation", detail)
	}
}

func TestGatherExplainsOneSyntheticAbsence(t *testing.T) {
	dir := t.TempDir()
	// The discrete device publishes the attributes the sampler reads (gpu_metrics
	// and the sysfs aliases), but not the fan: a real absence on a discrete GPU,
	// and the check must explain it rather than invent a reason.
	writeAttr(t, dir, "gpu_metrics", "1.2")
	writeAttr(t, dir, "gpu_busy_percent", "")
	writeAttr(t, dir, "mem_vram_used", "")
	writeAttr(t, dir, "mem_gtt_used", "")
	writeAttr(t, dir, "pp_dpm_sclk", "")
	writeAttr(t, dir, "temp1_input", "")
	writeAttr(t, dir, "power1_average", "")
	snap := syntheticSnapshot(dir, false)
	report := Gather(snap, "6.8.0-test")
	text := report.Render()
	if !strings.Contains(text, "fan") {
		t.Fatalf("report misses the fan metric:\\n%s", text)
	}
	if !strings.Contains(text, "not published") {
		t.Fatalf("report does not explain the fan:\\n%s", text)
	}
	if !strings.Contains(text, "APUs have no fan") {
		t.Fatalf("report misses why the fan is absent:\\n%s", text)
	}
	if !strings.Contains(text, "metrics available") {
		t.Fatalf("report misses the summary line:\\n%s", text)
	}
}

func TestGatherReportsFanOnDiscreteWithAttribute(t *testing.T) {
	dir := t.TempDir()
	// The device has hwmon (the fan source) but no fan sensor: the fan is
	// reported missing, and its note explains why it is not expected.
	writeAttr(t, dir, "hwmon", "")
	snap := syntheticSnapshot(dir, false)
	text := Gather(snap, "6.8.0-test").Render()
	if !strings.Contains(text, "fan") {
		t.Fatalf("report misses the fan metric:\n%s", text)
	}
	if !strings.Contains(text, "APUs have no fan") {
		t.Fatalf("report misses the fan explanation:\n%s", text)
	}
}

func TestRenderIsPlainASCIISafeToPaste(t *testing.T) {
	dir := t.TempDir()
	snap := syntheticSnapshot(dir, false)
	text := Gather(snap, "6.8.0-test").Render()
	for _, r := range text {
		if r > 127 {
			t.Fatalf("report contains non-ASCII rune %q; it must paste cleanly", r)
		}
	}
	if strings.Contains(text, "\x1b") {
		t.Fatal("report contains an escape sequence")
	}
}

func TestGatherNoDevicesStillRenders(t *testing.T) {
	report := Gather(model.GpuSnapshot{}, "6.8.0-test")
	text := report.Render()
	if !strings.Contains(text, "No amdgpu device") {
		t.Fatalf("empty report does not say so:\n%s", text)
	}
}

func TestKernelVersionParsesReleaseStrings(t *testing.T) {
	cases := map[string][2]int{
		"6.8.0-1-amd64": {6, 8},
		"6.10":          {6, 10},
		"5.15.0":        {5, 15},
		"garbage":       {0, 0},
		"":              {0, 0},
	}
	for release, want := range cases {
		if got := kernelVersion(release); got != want {
			t.Fatalf("kernelVersion(%q) = %v, want %v", release, got, want)
		}
	}
}

func writeAttr(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// syntheticSnapshot is one discrete (or integrated) GPU whose device
// directory is a temp dir, so every absent attribute is a synthetic
// absence the report has to explain.
func syntheticSnapshot(deviceDir string, integrated bool) model.GpuSnapshot {
	m := model.AmdgpuMetrics{
		Device: model.AmdgpuDevice{
			Index: 0, Card: "card0", BDF: "0000:0c:00.0",
			Name: "AMD Radeon Test", DeviceDir: deviceDir,
			Kind: model.DeviceDGPU,
		},
	}
	if integrated {
		m.Device.Kind = model.DeviceIGPU
	}
	return model.GpuSnapshot{Devices: []model.AmdgpuMetrics{m}}
}
