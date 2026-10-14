package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// write puts content at path, creating parent directories.
func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// hasWarning reports whether any warning contains sub.
func hasWarning(c Config, sub string) bool {
	for _, w := range c.Warnings {
		if strings.Contains(w, sub) {
			return true
		}
	}
	return false
}

func TestDefaults(t *testing.T) {
	c := Default()
	if c.IntervalMs != 1000 {
		t.Fatalf("interval_ms = %d, want 1000", c.IntervalMs)
	}
	if c.HistoryPoints != DefaultHistoryPoints {
		t.Fatalf("history_points = %d, want %d", c.HistoryPoints, DefaultHistoryPoints)
	}
	if c.IntervalS() != 1.0 {
		t.Fatalf("interval_s = %v, want 1.0", c.IntervalS())
	}
	if c.HistoryLength() != DefaultHistoryPoints {
		t.Fatalf("history_length = %d, want %d", c.HistoryLength(), DefaultHistoryPoints)
	}
	if len(c.Warnings) != 0 {
		t.Fatalf("defaults carry warnings: %v", c.Warnings)
	}
}

func TestMissingFileYieldsDefaultsWithAWarning(t *testing.T) {
	c := Load(filepath.Join(t.TempDir(), "absent.yaml"))
	if c.IntervalMs != 1000 {
		t.Fatalf("interval_ms = %d, want 1000 default", c.IntervalMs)
	}
	if !hasWarning(c, "not found") {
		t.Fatalf("warnings = %v, want a 'not found' warning", c.Warnings)
	}
}

func TestValidFileIsParsed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gputop.yaml")
	write(t, path, "interval_ms: 250\nhistory_points: 60\n")
	c := Load(path)
	if c.IntervalMs != 250 {
		t.Fatalf("interval_ms = %d, want 250", c.IntervalMs)
	}
	if c.HistoryLength() != 60 {
		t.Fatalf("history_length = %d, want 60", c.HistoryLength())
	}
	if len(c.Warnings) != 0 {
		t.Fatalf("warnings = %v, want none", c.Warnings)
	}
}

func TestUnknownKeyIsAWarningNotAnError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gputop.yaml")
	write(t, path, "interval_ms: 500\nfrom_the_future: 1\n")
	c := Load(path)
	if c.IntervalMs != 500 {
		t.Fatalf("interval_ms = %d, want 500", c.IntervalMs)
	}
	if !hasWarning(c, "from_the_future") {
		t.Fatalf("warnings = %v, want one naming from_the_future", c.Warnings)
	}
}

func TestUnknownSectionIsAWarning(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gputop.yaml")
	write(t, path, "diagnostics:\n  enabled: true\n")
	c := Load(path)
	if !hasWarning(c, "diagnostics") {
		t.Fatalf("warnings = %v, want one naming diagnostics", c.Warnings)
	}
}

func TestMalformedFileIsReported(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gputop.yaml")
	write(t, path, "interval_ms: 1\n  bad: indent\n:::\n")
	c := Load(path)
	if len(c.Warnings) == 0 {
		t.Fatal("a malformed file must be reported, not silently accepted")
	}
}

func TestOutOfRangeIntervalIsClamped(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gputop.yaml")
	write(t, path, "interval_ms: 5\n")
	c := Load(path)
	if c.IntervalMs != MinIntervalMs {
		t.Fatalf("interval_ms = %d, want floor %d", c.IntervalMs, MinIntervalMs)
	}
	if !hasWarning(c, "out of range") {
		t.Fatalf("warnings = %v, want an 'out of range' warning", c.Warnings)
	}
}

func TestInvalidKindFallsBackToAuto(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gputop.yaml")
	write(t, path, "gpu:\n  kind: sideways\n")
	c := Load(path)
	if c.Gpu.Kind != "auto" {
		t.Fatalf("gpu.kind = %q, want auto", c.Gpu.Kind)
	}
	if !hasWarning(c, "gpu.kind") {
		t.Fatalf("warnings = %v, want one naming gpu.kind", c.Warnings)
	}
}

func TestGpuNamesMapping(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gputop.yaml")
	write(t, path, "gpu:\n  names:\n    \"0000:0c:00.0\": My GPU\n")
	c := Load(path)
	if c.Gpu.Names["0000:0c:00.0"] != "My GPU" {
		t.Fatalf("gpu.names = %v, want the parsed mapping", c.Gpu.Names)
	}
}

func TestGpuDevicesList(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gputop.yaml")
	write(t, path, "gpu:\n  devices:\n    - card0\n    - \"0000:03:00.0\"\n")
	c := Load(path)
	if len(c.Gpu.Devices) != 2 || c.Gpu.Devices[0] != "card0" {
		t.Fatalf("gpu.devices = %v, want [card0 0000:03:00.0]", c.Gpu.Devices)
	}
}

func TestGpuDevicesRejectsABareString(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gputop.yaml")
	write(t, path, "gpu:\n  devices: card0\n")
	c := Load(path)
	if len(c.Gpu.Devices) != 0 {
		t.Fatalf("gpu.devices = %v, want empty (a bare string is not a list)", c.Gpu.Devices)
	}
	if !hasWarning(c, "gpu.devices") {
		t.Fatalf("warnings = %v, want one naming gpu.devices", c.Warnings)
	}
}

func TestNumberThatIsNotANumberIsAWarningNotACrash(t *testing.T) {
	// A string where a number belongs must fall back to the default with a
	// warning, never reach a comparison that raises.
	cases := []struct {
		body     string
		get      func(Config) int
		expected int
	}{
		{"interval_ms: fast\n", func(c Config) int { return c.IntervalMs }, 1000},
		{"history_points: many\n", func(c Config) int { return c.HistoryPoints }, 300},
		{"process:\n  max_rows: lots\n", func(c Config) int { return c.Process.MaxRows }, 20},
		{"ui:\n  graph_height: tall\n", func(c Config) int { return c.UI.GraphHeight }, 0},
	}
	for _, tc := range cases {
		path := filepath.Join(t.TempDir(), "gputop.yaml")
		write(t, path, tc.body)
		c := Load(path)
		if got := tc.get(c); got != tc.expected {
			t.Fatalf("body %q: got %d, want %d", tc.body, got, tc.expected)
		}
		if len(c.Warnings) == 0 {
			t.Fatalf("body %q: want a warning", tc.body)
		}
	}
}

func TestABooleanIsNotANumber(t *testing.T) {
	// interval_ms: true is 1 ms and YAML's true is not an int; it must be
	// rejected as a boolean with a warning, not coerced to 1.
	path := filepath.Join(t.TempDir(), "gputop.yaml")
	write(t, path, "interval_ms: true\n")
	c := Load(path)
	if c.IntervalMs != 1000 {
		t.Fatalf("interval_ms = %d, want 1000 default", c.IntervalMs)
	}
	if !hasWarning(c, "boolean") {
		t.Fatalf("warnings = %v, want one mentioning a boolean", c.Warnings)
	}
}

func TestThemeValidation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gputop.yaml")
	write(t, path, "ui:\n  theme: neon\n")
	c := Load(path)
	if c.UI.Theme != "default" {
		t.Fatalf("ui.theme = %q, want default fallback", c.UI.Theme)
	}
	if !hasWarning(c, "ui.theme") {
		t.Fatalf("warnings = %v, want one naming ui.theme", c.Warnings)
	}
}

func TestAlertsThresholdsFlowToModel(t *testing.T) {
	c := Default()
	th := c.Alerts.Thresholds()
	if th.TempC != 90.0 || th.PowerPercent != 95.0 || th.VramPercent != 92.0 {
		t.Fatalf("thresholds = %+v, want the defaults 90/95/92", th)
	}
}

func TestAlertsClampOutOfRange(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gputop.yaml")
	write(t, path, "alerts:\n  temp_c: 500\n  power_percent: 200\n")
	c := Load(path)
	if c.Alerts.TempC != 125.0 {
		t.Fatalf("alerts.temp_c = %v, want clamp to 125", c.Alerts.TempC)
	}
	if c.Alerts.PowerPercent != 100.0 {
		t.Fatalf("alerts.power_percent = %v, want clamp to 100", c.Alerts.PowerPercent)
	}
	if !hasWarning(c, "alerts.temp_c") {
		t.Fatalf("warnings = %v, want one naming alerts.temp_c", c.Warnings)
	}
}

func TestStateSectionKeysAreTranslated(t *testing.T) {
	// The state: block uses file keys (process_sort, process_filter) that
	// differ from the struct field names (sort, filter); the loader maps them.
	path := filepath.Join(t.TempDir(), "gputop.yaml")
	write(t, path, "state:\n  theme: dracula\n  process_sort: gfx\n  process_filter: game\n")
	c := Load(path)
	if c.State.Theme == nil || *c.State.Theme != "dracula" {
		t.Fatalf("state.theme = %v, want dracula", c.State.Theme)
	}
	if c.State.Sort == nil || *c.State.Sort != "gfx" {
		t.Fatalf("state.sort = %v, want gfx (from process_sort)", c.State.Sort)
	}
	if c.State.Filter != "game" {
		t.Fatalf("state.filter = %q, want game (from process_filter)", c.State.Filter)
	}
}

func TestStateWithUnknownThemeIsIgnored(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gputop.yaml")
	write(t, path, "state:\n  theme: does-not-exist\n")
	c := Load(path)
	if c.State.Theme != nil {
		t.Fatalf("state.theme = %v, want nil (unknown value ignored)", c.State.Theme)
	}
}

func TestStateSectionOfTheWrongShapeStillStarts(t *testing.T) {
	bodies := []string{
		"ui:\n  theme:\n    - a\n    - b\n",
		"state:\n  theme:\n    nested: 1\n",
		"state:\n  interval_ms: fast\n",
		"state:\n  gpu_index: first\n",
	}
	for _, body := range bodies {
		path := filepath.Join(t.TempDir(), "gputop.yaml")
		write(t, path, body)
		// Must not panic and must return a usable Config.
		c := Load(path)
		if c.IntervalS() <= 0 {
			t.Fatalf("body %q produced a non-positive interval %v", body, c.IntervalS())
		}
	}
}
