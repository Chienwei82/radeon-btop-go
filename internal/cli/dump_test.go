package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/gputop/gputop-go/internal/config"
	"github.com/gputop/gputop-go/internal/model"
	"github.com/gputop/gputop-go/internal/sampler"
)

func ptr[T any](v T) *T { return &v }

// testSnapshot builds a fully populated snapshot so every optional field is
// present at least once and the null-vs-absent distinction can be checked.
func testSnapshot() model.GpuSnapshot {
	device := model.AmdgpuDevice{
		Index:    0,
		Card:     "card1",
		Render:   ptr("renderD128"),
		BDF:      "0000:0c:00.0",
		VendorID: 0x1002,
		DeviceID: 0x73BF,
		Kind:     model.DeviceDGPU,
		Name:     "AMD Radeon RX 6800",
		BootVGA:  true,
	}
	metrics := model.AmdgpuMetrics{
		Device:           device,
		Sequence:         7,
		TimestampNs:      123,
		IntervalS:        1.0,
		GpuBusyPercent:   ptr(42.5),
		MemBusyPercent:   nil,
		VRAM:             model.MemoryPool{Used: ptr(int64(4 << 30)), Total: ptr(int64(16 << 30)), Source: "sysfs"},
		GTT:              model.MemoryPool{},
		SCLK:             model.Clock{Current: ptr(2104), Maximum: ptr(2600), Source: "dpm"},
		MCLK:             model.Clock{},
		SocClk:           model.Clock{},
		VoltagesMv:       []model.VoltageReading{{Name: "vddgfx", MV: 1100}, {Name: "vddnb", MV: 900}},
		Temperatures:     []model.Temperature{{Label: "edge", Celsius: ptr(51.0), Source: "gpu_metrics"}, {Label: "junction", Source: "gpu_metrics"}},
		Power:            model.Power{DrawW: ptr(33.0), CapW: ptr(203.0), EnergyM: ptr(int64(1234)), Source: "gpu_metrics"},
		Fan:              model.Fan{RPM: ptr(1500), MaxRPM: ptr(3000), PWM: ptr(50.0)},
		PerformanceLevel: ptr("auto"),
		PowerProfile:     ptr("3D_FULL_SCREEN"),
		Throttle:         &model.ThrottleInfo{Raw: 5, Active: []string{"sw_thermal", "hw_thermal"}},
		PCIE:             &model.PcieLink{Width: 16, Speed: 80},
		MetricsABI:       ptr("v1.3"),
		Blocks: &model.GpuBlocks{
			Blocks:      []model.BlockReading{{Key: "gpu", Label: "Graphics pipe", Percent: 12.5}, {Key: "ee", Label: "Event Engine", Percent: 3}},
			AuxBlocks:   []model.BlockReading{{Key: "uvd", Label: "UVD", Percent: 0}},
			Clocks:      []model.ClockReading{{Key: "sclk", Label: "Shader Clock", Percent: ptr(80.0), Mhz: ptr(2104)}},
			TimestampNs: 999,
		},
		ODC: model.OdcVoltage{
			Present:        true,
			VddgfxOffsetMv: ptr(0),
			Domains: []model.OdvDomain{{
				Name:    "sclk",
				Minimum: ptr(500),
				Maximum: ptr(2600),
				Unit:    model.OdvMegahertz,
				Entries: []model.OdvEntry{{State: 0, Value: 2104, Unit: model.OdvMegahertz}},
			}},
		},
		Profiles: model.PowerProfileTable{
			Present:  true,
			Profiles: []model.PowerProfileEntry{{Index: 0, Name: "BOOTUP_DEFAULT"}, {Index: 2, Name: "3D_FULL_SCREEN", Active: true}},
		},
	}
	process := model.GpuProcess{
		PID: 4242, Name: "ffmpeg", User: "david", BDF: "0000:0c:00.0", ClientID: 9,
		Engines:  []model.EngineUsage{{Engine: "dec", TotalNs: 500, DeltaNs: 100, WindowNs: 1000}},
		VramUsed: ptr(int64(1024)), GttUsed: ptr(int64(512)), CpuUsed: ptr(int64(256)),
	}
	return model.GpuSnapshot{
		Sequence:            7,
		TimestampNs:         123,
		Devices:             []model.AmdgpuMetrics{metrics},
		Processes:           []model.GpuProcess{process},
		VisibleProcessCount: 1,
		TotalProcessCount:   3,
		Stats:               model.SamplerStats{Ticks: 2, TickErrors: 0, LastDurationMs: 1.5, AvgDurationMs: 1.25},
		Source:              model.SourceReport{MetricsABI: "v1.3"},
		Warnings:            []string{"boom"},
		BlocksStatus:        model.BlocksDisabled,
		BlocksHint:          "blocks off (--blocks to enable)",
	}
}

// encode marshals a payload to compact JSON bytes.
func encode(t *testing.T, payload any) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := Encode(&buf, payload, false); err != nil {
		t.Fatalf("Encode: %v", err)
	}
	return buf.Bytes()
}

// objectKeys returns the keys of a JSON object in the order they appear.
func objectKeys(t *testing.T, raw []byte) []string {
	t.Helper()
	dec := json.NewDecoder(bytes.NewReader(raw))
	tok, err := dec.Token()
	if err != nil {
		t.Fatalf("token: %v", err)
	}
	if d, ok := tok.(json.Delim); !ok || d != '{' {
		t.Fatalf("want object, got %v", tok)
	}
	var keys []string
	for dec.More() {
		key, err := dec.Token()
		if err != nil {
			t.Fatalf("key token: %v", err)
		}
		keys = append(keys, key.(string))
		var skip json.RawMessage
		if err := dec.Decode(&skip); err != nil {
			t.Fatalf("skip value: %v", err)
		}
	}
	return keys
}

// rawField navigates an object to one member and returns its raw JSON.
func rawField(t *testing.T, raw []byte, key string) json.RawMessage {
	t.Helper()
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("unmarshal object: %v", err)
	}
	v, ok := m[key]
	if !ok {
		t.Fatalf("missing key %q in %s", key, raw)
	}
	return v
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestSnapshotTopLevelKeyOrder(t *testing.T) {
	cfg := config.Default()
	raw := encode(t, Snapshot(testSnapshot(), &cfg))
	want := []string{
		"sequence", "timestamp_ns", "metrics_abi", "using_binary_metrics",
		"devices", "processes", "process_visibility", "stats", "warnings",
		"blocks", "alerts",
	}
	if got := objectKeys(t, raw); !equalStrings(got, want) {
		t.Fatalf("top-level keys = %v, want %v", got, want)
	}
}

func TestDeviceKeyOrderAndDeviceID(t *testing.T) {
	cfg := config.Default()
	raw := encode(t, Snapshot(testSnapshot(), &cfg))
	var devices []json.RawMessage
	if err := json.Unmarshal(rawField(t, raw, "devices"), &devices); err != nil {
		t.Fatalf("devices: %v", err)
	}
	if len(devices) != 1 {
		t.Fatalf("devices len = %d, want 1", len(devices))
	}
	want := []string{
		"index", "card", "render", "bdf", "name", "kind", "integrated", "device_id",
		"boot_vga", "metrics_abi", "gpu_busy_percent", "mem_busy_percent", "vram", "gtt",
		"sclk", "mclk", "socclk", "temperatures", "voltages_mv", "power", "fan",
		"performance_level", "power_profile", "throttle", "pcie", "blocks", "odc", "power_profiles",
	}
	if got := objectKeys(t, devices[0]); !equalStrings(got, want) {
		t.Fatalf("device keys = %v, want %v", got, want)
	}
	var id string
	if err := json.Unmarshal(rawField(t, devices[0], "device_id"), &id); err != nil {
		t.Fatalf("device_id: %v", err)
	}
	if id != "0x73bf" {
		t.Fatalf("device_id = %q, want 0x73bf", id)
	}
}

func TestMemoryPoolHasPercentAndNoSource(t *testing.T) {
	cfg := config.Default()
	raw := encode(t, Snapshot(testSnapshot(), &cfg))
	var devices []json.RawMessage
	if err := json.Unmarshal(rawField(t, raw, "devices"), &devices); err != nil {
		t.Fatalf("devices: %v", err)
	}
	vram := rawField(t, devices[0], "vram")
	if got, want := objectKeys(t, vram), []string{"used", "total", "percent"}; !equalStrings(got, want) {
		t.Fatalf("vram keys = %v, want %v", got, want)
	}
	var pool struct {
		Percent *float64 `json:"percent"`
	}
	if err := json.Unmarshal(vram, &pool); err != nil {
		t.Fatalf("vram: %v", err)
	}
	if pool.Percent == nil || *pool.Percent != 25 {
		t.Fatalf("vram percent = %v, want 25", pool.Percent)
	}
	// An empty pool stays an object with all-null readings, never omitted.
	gtt := rawField(t, devices[0], "gtt")
	if !strings.Contains(string(gtt), `"used":null`) {
		t.Fatalf("gtt = %s, want used:null", gtt)
	}
}

func TestVoltagesAreOrderedObject(t *testing.T) {
	cfg := config.Default()
	raw := encode(t, Snapshot(testSnapshot(), &cfg))
	var devices []json.RawMessage
	if err := json.Unmarshal(rawField(t, raw, "devices"), &devices); err != nil {
		t.Fatalf("devices: %v", err)
	}
	voltages := rawField(t, devices[0], "voltages_mv")
	if got, want := objectKeys(t, voltages), []string{"vddgfx", "vddnb"}; !equalStrings(got, want) {
		t.Fatalf("voltages_mv keys = %v, want %v", got, want)
	}
}

func TestBlocksShapeAndOrder(t *testing.T) {
	cfg := config.Default()
	raw := encode(t, Snapshot(testSnapshot(), &cfg))
	var devices []json.RawMessage
	if err := json.Unmarshal(rawField(t, raw, "devices"), &devices); err != nil {
		t.Fatalf("devices: %v", err)
	}
	blocks := rawField(t, devices[0], "blocks")
	if got, want := objectKeys(t, blocks), []string{"percent", "busiest", "clocks", "timestamp_ns"}; !equalStrings(got, want) {
		t.Fatalf("blocks keys = %v, want %v", got, want)
	}
	// Pipeline blocks come first, then aux blocks.
	if got, want := objectKeys(t, rawField(t, blocks, "percent")), []string{"gpu", "ee", "uvd"}; !equalStrings(got, want) {
		t.Fatalf("percent keys = %v, want %v", got, want)
	}
	if got, want := objectKeys(t, rawField(t, blocks, "clocks")), []string{"sclk"}; !equalStrings(got, want) {
		t.Fatalf("clocks keys = %v, want %v", got, want)
	}
	var busiest string
	if err := json.Unmarshal(rawField(t, blocks, "busiest"), &busiest); err != nil {
		t.Fatalf("busiest: %v", err)
	}
	if busiest != "gpu" {
		t.Fatalf("busiest = %q, want gpu", busiest)
	}
}

func TestBlocksNullWhenAbsent(t *testing.T) {
	cfg := config.Default()
	snap := testSnapshot()
	snap.Devices[0].Blocks = nil
	raw := encode(t, Snapshot(snap, &cfg))
	var devices []json.RawMessage
	if err := json.Unmarshal(rawField(t, raw, "devices"), &devices); err != nil {
		t.Fatalf("devices: %v", err)
	}
	if got := strings.TrimSpace(string(rawField(t, devices[0], "blocks"))); got != "null" {
		t.Fatalf("blocks = %s, want null", got)
	}
}

func TestODCEntriesAreArrays(t *testing.T) {
	cfg := config.Default()
	raw := encode(t, Snapshot(testSnapshot(), &cfg))
	var devices []json.RawMessage
	if err := json.Unmarshal(rawField(t, raw, "devices"), &devices); err != nil {
		t.Fatalf("devices: %v", err)
	}
	odc := rawField(t, devices[0], "odc")
	if got, want := objectKeys(t, odc), []string{"present", "vddgfx_offset_mv", "domains"}; !equalStrings(got, want) {
		t.Fatalf("odc keys = %v, want %v", got, want)
	}
	var domains []json.RawMessage
	if err := json.Unmarshal(rawField(t, odc, "domains"), &domains); err != nil {
		t.Fatalf("domains: %v", err)
	}
	var entries [][]any
	if err := json.Unmarshal(rawField(t, domains[0], "entries"), &entries); err != nil {
		t.Fatalf("entries: %v", err)
	}
	if len(entries) != 1 || len(entries[0]) != 3 {
		t.Fatalf("entries = %v, want one 3-element array", entries)
	}
	if entries[0][2] != "MHz" {
		t.Fatalf("entry unit = %v, want MHz", entries[0][2])
	}
}

func TestThrottleActiveIsSorted(t *testing.T) {
	cfg := config.Default()
	raw := encode(t, Snapshot(testSnapshot(), &cfg))
	var devices []json.RawMessage
	if err := json.Unmarshal(rawField(t, raw, "devices"), &devices); err != nil {
		t.Fatalf("devices: %v", err)
	}
	var throttle struct {
		Active []string `json:"active"`
	}
	if err := json.Unmarshal(rawField(t, devices[0], "throttle"), &throttle); err != nil {
		t.Fatalf("throttle: %v", err)
	}
	if got, want := throttle.Active, []string{"hw_thermal", "sw_thermal"}; !equalStrings(got, want) {
		t.Fatalf("active = %v, want %v", got, want)
	}
}

func TestAlertsReportABreach(t *testing.T) {
	cfg := config.Default()
	snap := testSnapshot()
	snap.Devices[0].Temperatures = []model.Temperature{{Label: "junction", Celsius: ptr(95.0), Source: "gpu_metrics"}}
	raw := encode(t, Snapshot(snap, &cfg))
	alerts := rawField(t, raw, "alerts")
	var got struct {
		Enabled  bool   `json:"enabled"`
		Level    string `json:"level"`
		Breaches []struct {
			Metric  string `json:"metric"`
			Reading string `json:"reading"`
			Limit   string `json:"limit"`
		} `json:"breaches"`
		Thresholds struct {
			TempC float64 `json:"temp_c"`
		} `json:"thresholds"`
	}
	if err := json.Unmarshal(alerts, &got); err != nil {
		t.Fatalf("alerts: %v", err)
	}
	if !got.Enabled || got.Level != "alert" {
		t.Fatalf("enabled/level = %v/%q, want true/alert", got.Enabled, got.Level)
	}
	if len(got.Breaches) != 1 {
		t.Fatalf("breaches = %v, want one", got.Breaches)
	}
	b := got.Breaches[0]
	if b.Metric != "temp junction" || b.Reading != "95C" || b.Limit != "90C" {
		t.Fatalf("breach = %+v, want temp junction 95C > 90C", b)
	}
	if got.Thresholds.TempC != 90 {
		t.Fatalf("thresholds.temp_c = %v, want 90", got.Thresholds.TempC)
	}
}

func TestAlertsDisabledDropsThresholds(t *testing.T) {
	cfg := config.Default()
	cfg.Alerts.Enabled = false
	raw := encode(t, Snapshot(testSnapshot(), &cfg))
	alerts := rawField(t, raw, "alerts")
	if got, want := objectKeys(t, alerts), []string{"enabled", "level", "breaches"}; !equalStrings(got, want) {
		t.Fatalf("alerts keys = %v, want %v", got, want)
	}
	if !strings.Contains(string(alerts), `"breaches":[]`) {
		t.Fatalf("alerts = %s, want empty breaches", alerts)
	}
}

func TestAlertsNullWithoutConfig(t *testing.T) {
	raw := encode(t, Snapshot(testSnapshot(), nil))
	if got := strings.TrimSpace(string(rawField(t, raw, "alerts"))); got != "null" {
		t.Fatalf("alerts = %s, want null", got)
	}
}

func TestSnapshotNullsForAbsentReadings(t *testing.T) {
	cfg := config.Default()
	raw := encode(t, Snapshot(testSnapshot(), &cfg))
	for _, needle := range []string{`"mem_busy_percent":null`} {
		if !strings.Contains(string(raw), needle) {
			t.Fatalf("snapshot missing %s", needle)
		}
	}
	// An all-empty second device must still carry every field as null/empty.
	snap := testSnapshot()
	snap.Devices = append(snap.Devices, model.AmdgpuMetrics{Device: model.AmdgpuDevice{Index: 1, Card: "card2", BDF: "0000:0d:00.0", Kind: model.DeviceDGPU}})
	raw = encode(t, Snapshot(snap, &cfg))
	var devices []json.RawMessage
	if err := json.Unmarshal(rawField(t, raw, "devices"), &devices); err != nil {
		t.Fatalf("devices: %v", err)
	}
	if got := strings.TrimSpace(string(rawField(t, devices[1], "metrics_abi"))); got != "null" {
		t.Fatalf("metrics_abi = %s, want null", got)
	}
	if got := strings.TrimSpace(string(rawField(t, devices[1], "throttle"))); got != "null" {
		t.Fatalf("throttle = %s, want null", got)
	}
}

func TestProcessShapeAndMemoryTotal(t *testing.T) {
	cfg := config.Default()
	raw := encode(t, Snapshot(testSnapshot(), &cfg))
	var procs []json.RawMessage
	if err := json.Unmarshal(rawField(t, raw, "processes"), &procs); err != nil {
		t.Fatalf("processes: %v", err)
	}
	want := []string{
		"pid", "name", "user", "bdf", "client_id", "engine_percent",
		"engines", "vram_used", "gtt_used", "cpu_used", "memory_used",
	}
	if got := objectKeys(t, procs[0]); !equalStrings(got, want) {
		t.Fatalf("process keys = %v, want %v", got, want)
	}
	var p struct {
		MemoryUsed int64 `json:"memory_used"`
	}
	if err := json.Unmarshal(procs[0], &p); err != nil {
		t.Fatalf("process: %v", err)
	}
	if p.MemoryUsed != 1024+512+256 {
		t.Fatalf("memory_used = %d, want 1792", p.MemoryUsed)
	}
}

func TestStatsAndVisibilityShape(t *testing.T) {
	cfg := config.Default()
	raw := encode(t, Snapshot(testSnapshot(), &cfg))
	if got, want := objectKeys(t, rawField(t, raw, "stats")), []string{"ticks", "tick_errors", "last_duration_ms", "avg_duration_ms"}; !equalStrings(got, want) {
		t.Fatalf("stats keys = %v, want %v", got, want)
	}
	vis := rawField(t, raw, "process_visibility")
	if got, want := objectKeys(t, vis), []string{"visible", "total", "partial"}; !equalStrings(got, want) {
		t.Fatalf("process_visibility keys = %v, want %v", got, want)
	}
	var v struct {
		Visible int  `json:"visible"`
		Total   int  `json:"total"`
		Partial bool `json:"partial"`
	}
	if err := json.Unmarshal(vis, &v); err != nil {
		t.Fatalf("visibility: %v", err)
	}
	if v.Visible != 1 || v.Total != 3 || !v.Partial {
		t.Fatalf("visibility = %+v, want 1/3/true", v)
	}
	var abi string
	if err := json.Unmarshal(rawField(t, raw, "metrics_abi"), &abi); err != nil {
		t.Fatalf("metrics_abi: %v", err)
	}
	if abi != "v1.3" {
		t.Fatalf("metrics_abi = %q, want v1.3", abi)
	}
	var usingBinary bool
	if err := json.Unmarshal(rawField(t, raw, "using_binary_metrics"), &usingBinary); err != nil {
		t.Fatalf("using_binary_metrics: %v", err)
	}
	if !usingBinary {
		t.Fatal("using_binary_metrics = false, want true")
	}
}

func TestDevicesPayloadWithoutHardware(t *testing.T) {
	var buf bytes.Buffer
	status, err := Devices(&buf, OneShot{Sampler: sampler.SamplerOptions{DrmRoot: "/nonexistent/drm"}})
	if err != nil {
		t.Fatalf("Devices: %v", err)
	}
	if status != ExitNoDevice {
		t.Fatalf("status = %d, want %d", status, ExitNoDevice)
	}
	if got, want := objectKeys(t, buf.Bytes()), []string{"count", "devices", "warnings"}; !equalStrings(got, want) {
		t.Fatalf("keys = %v, want %v", got, want)
	}
	var got struct {
		Count    int               `json:"count"`
		Devices  []json.RawMessage `json:"devices"`
		Warnings []string          `json:"warnings"`
	}
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Count != 0 || len(got.Devices) != 0 {
		t.Fatalf("count/devices = %d/%d, want 0/0", got.Count, len(got.Devices))
	}
	if len(got.Warnings) != 0 {
		t.Fatalf("warnings = %v, want none", got.Warnings)
	}
}

func TestDumpWithoutHardwareReportsReason(t *testing.T) {
	var buf bytes.Buffer
	status, err := Dump(&buf, OneShot{Sampler: sampler.SamplerOptions{DrmRoot: "/nonexistent/drm"}})
	if err != nil {
		t.Fatalf("Dump: %v", err)
	}
	if status != ExitNoDevice {
		t.Fatalf("status = %d, want %d", status, ExitNoDevice)
	}
	if got, want := objectKeys(t, buf.Bytes()), []string{"devices", "processes", "warnings"}; !equalStrings(got, want) {
		t.Fatalf("keys = %v, want %v", got, want)
	}
	var got struct {
		Devices   []json.RawMessage `json:"devices"`
		Processes []json.RawMessage `json:"processes"`
		Warnings  []string          `json:"warnings"`
	}
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got.Devices) != 0 || len(got.Processes) != 0 {
		t.Fatalf("devices/processes = %d/%d, want 0/0", len(got.Devices), len(got.Processes))
	}
	if len(got.Warnings) != 1 || !strings.Contains(got.Warnings[0], "/nonexistent/drm") {
		t.Fatalf("warnings = %v, want the drm root named", got.Warnings)
	}
}

func TestEncodePrettyAndNoHTMLEscape(t *testing.T) {
	var buf bytes.Buffer
	if err := Encode(&buf, map[string]any{"w": "<a&b>"}, true); err != nil {
		t.Fatalf("Encode: %v", err)
	}
	out := buf.String()
	if !strings.HasSuffix(out, "}\n") {
		t.Fatalf("output %q must end with a newline", out)
	}
	if !strings.Contains(out, "\n  ") {
		t.Fatalf("pretty output not indented: %q", out)
	}
	if !strings.Contains(out, "<a&b>") {
		t.Fatalf("HTML must not be escaped: %q", out)
	}
}

func TestStartupWarningsCombinesConfigAndFilter(t *testing.T) {
	cfg := config.Default()
	cfg.Warnings = []string{"unknown key ignored"}
	got := StartupWarnings(&cfg, "gpu.devices: no such device: card9")
	want := []string{"unknown key ignored", "gpu.devices: no such device: card9"}
	if !equalStrings(got, want) {
		t.Fatalf("warnings = %v, want %v", got, want)
	}
	if got := StartupWarnings(nil, ""); got == nil || len(got) != 0 {
		t.Fatalf("empty warnings = %v, want a non-nil empty slice", got)
	}
}
