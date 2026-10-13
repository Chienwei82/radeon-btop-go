package readers

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gputop/gputop-go/internal/model"
)

func writeBytes(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func dev(dir string) model.AmdgpuDevice { return model.AmdgpuDevice{DeviceDir: dir} }

// A pre-5.19 kernel has no gpu_metrics; the sysfs/DPM path must carry the sample.
func TestSampleDeviceSysfsFallback(t *testing.T) {
	dir := t.TempDir()
	writeAttr(t, filepath.Join(dir, "pp_dpm_sclk"), "0: 500Mhz *\n1: 2400Mhz \n")
	writeAttr(t, filepath.Join(dir, "mem_info_vis_vram_used"), "1048576")
	writeAttr(t, filepath.Join(dir, "mem_info_vis_vram_total"), "8589934592")
	writeAttr(t, filepath.Join(dir, "gpu_busy_percent"), "42")

	m := SampleDevice(dev(dir), 1, 0, 1.0)

	if m.MetricsABI != nil {
		t.Fatalf("no binary table => abi nil, got %v", *m.MetricsABI)
	}
	if m.SCLK.Source != "dpm" {
		t.Fatalf("sclk source = %q, want dpm", m.SCLK.Source)
	}
	if m.SCLK.Current == nil || *m.SCLK.Current != 500 {
		t.Fatalf("sclk current = %v, want 500 (starred row)", m.SCLK.Current)
	}
	if m.SCLK.Maximum == nil || *m.SCLK.Maximum != 2400 {
		t.Fatalf("sclk maximum = %v, want 2400", m.SCLK.Maximum)
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
}

// gpu_metrics wins over sysfs for clocks/activity/power/pcie, but the clock
// ceiling still comes from the sysfs/DPM chain, never from the binary table.
func TestSampleDeviceBinaryWins(t *testing.T) {
	dir := t.TempDir()
	writeAttr(t, filepath.Join(dir, "current_sclk"), "999")
	abi := ABIRegistry[[2]int{1, 3}]
	blob := buildTable(t, abi, map[string]uint64{
		"temperature_edge":     49,
		"average_gfx_activity": 12,
		"average_socket_power": 33,
		"current_gfxclk":       500,
		"current_uclk":         1000,
		"pcie_link_width":      16,
		"pcie_link_speed":      80,
	}, -1, 0)
	writeBytes(t, filepath.Join(dir, "gpu_metrics"), blob)

	m := SampleDevice(dev(dir), 1, 0, 1.0)

	if m.MetricsABI == nil || *m.MetricsABI != "v1.3" {
		t.Fatalf("abi = %v, want v1.3", m.MetricsABI)
	}
	if m.GpuBusyPercent == nil || *m.GpuBusyPercent != 12 {
		t.Fatalf("gpu busy = %v, want 12 (binary)", m.GpuBusyPercent)
	}
	if m.SCLK.Source != "gpu_metrics" || m.SCLK.Current == nil || *m.SCLK.Current != 500 {
		t.Fatalf("sclk = (%q,%v), want gpu_metrics/500", m.SCLK.Source, m.SCLK.Current)
	}
	if m.SCLK.Maximum != nil {
		t.Fatalf("sclk maximum must stay nil (never from binary), got %v", *m.SCLK.Maximum)
	}
	if len(m.Temperatures) == 0 || *m.Temperatures[0].Celsius != 49 || m.Temperatures[0].Source != "gpu_metrics" {
		t.Fatalf("temps = %v, want edge=49 gpu_metrics", m.Temperatures)
	}
	if m.Power.Source != "gpu_metrics" || m.Power.DrawW == nil || *m.Power.DrawW != 33 {
		t.Fatalf("power = (%q,%v), want gpu_metrics/33", m.Power.Source, m.Power.DrawW)
	}
	if m.PCIE == nil || m.PCIE.Describe() != "Gen3 x16" {
		t.Fatalf("pcie = %v, want Gen3 x16", m.PCIE)
	}
}

// hwmon and gpu_metrics merge: the binary wins where they overlap, but a
// sensor only hwmon knows about (a VRM rail) is kept.
func TestSampleDeviceTemperatureMerge(t *testing.T) {
	dir := t.TempDir()
	hw := filepath.Join(dir, "hwmon", "hwmon0")
	writeAttr(t, filepath.Join(hw, "temp1_input"), "51000")
	writeAttr(t, filepath.Join(hw, "temp1_label"), "edge")
	writeAttr(t, filepath.Join(hw, "temp2_input"), "60000")
	writeAttr(t, filepath.Join(hw, "temp2_label"), "soc")
	abi := ABIRegistry[[2]int{1, 3}]
	blob := buildTable(t, abi, map[string]uint64{
		"temperature_edge":    49,
		"temperature_hotspot": 56,
		"temperature_mem":     SentinelU16,
	}, -1, 0)
	writeBytes(t, filepath.Join(dir, "gpu_metrics"), blob)
	device := model.AmdgpuDevice{DeviceDir: dir, HwmonDir: &hw}

	m := SampleDevice(device, 1, 0, 1.0)

	byLabel := map[string]model.Temperature{}
	for _, temp := range m.Temperatures {
		byLabel[temp.Label] = temp
	}
	if e := byLabel["edge"]; e.Celsius == nil || *e.Celsius != 49 || e.Source != "gpu_metrics" {
		t.Fatalf("edge = %v, want 49/gpu_metrics", e)
	}
	if j := byLabel["junction"]; j.Celsius == nil || *j.Celsius != 56 {
		t.Fatalf("junction = %v, want 56", j)
	}
	if v := byLabel["soc"]; v.Celsius == nil || *v.Celsius != 60 || v.Source != "hwmon" {
		t.Fatalf("soc = %v, want 60/hwmon (hwmon-only kept)", v)
	}
	wantOrder := []string{"edge", "junction", "soc"}
	if len(m.Temperatures) != len(wantOrder) {
		t.Fatalf("temps = %v, want %v", m.Temperatures, wantOrder)
	}
	for i, label := range wantOrder {
		if m.Temperatures[i].Label != label {
			t.Fatalf("order[%d] = %q, want %q", i, m.Temperatures[i].Label, label)
		}
	}
}

// hwmon power1_cap always provides the cap; the draw comes from the binary.
func TestSampleDevicePowerCapFromHwmon(t *testing.T) {
	dir := t.TempDir()
	hw := filepath.Join(dir, "hwmon", "hwmon0")
	writeAttr(t, filepath.Join(hw, "power1_cap"), "200000000")
	abi := ABIRegistry[[2]int{1, 3}]
	blob := buildTable(t, abi, map[string]uint64{"average_socket_power": 33}, -1, 0)
	writeBytes(t, filepath.Join(dir, "gpu_metrics"), blob)
	device := model.AmdgpuDevice{DeviceDir: dir, HwmonDir: &hw}

	m := SampleDevice(device, 1, 0, 1.0)

	if m.Power.DrawW == nil || *m.Power.DrawW != 33 || m.Power.Source != "gpu_metrics" {
		t.Fatalf("draw = (%q,%v), want gpu_metrics/33", m.Power.Source, m.Power.DrawW)
	}
	if m.Power.CapW == nil || *m.Power.CapW != 200 {
		t.Fatalf("cap = %v, want 200 (hwmon)", m.Power.CapW)
	}
}

// Fan rpm/pwm prefer the binary; the ceiling always comes from hwmon.
func TestSampleDeviceFanBinaryWins(t *testing.T) {
	dir := t.TempDir()
	hw := filepath.Join(dir, "hwmon", "hwmon0")
	writeAttr(t, filepath.Join(hw, "fan1_input"), "1000")
	writeAttr(t, filepath.Join(hw, "fan1_max"), "3000")
	writeAttr(t, filepath.Join(hw, "pwm1"), "128")
	abi := ABIRegistry[[2]int{1, 3}]
	// v1.3 has current_fan_speed but no fan_pwm field (that is v2.x), so the
	// PWM must fall back to hwmon (128/255*100) even with a binary table.
	blob := buildTable(t, abi, map[string]uint64{"current_fan_speed": 1500}, -1, 0)
	writeBytes(t, filepath.Join(dir, "gpu_metrics"), blob)
	device := model.AmdgpuDevice{DeviceDir: dir, HwmonDir: &hw}

	m := SampleDevice(device, 1, 0, 1.0)

	if m.Fan.RPM == nil || *m.Fan.RPM != 1500 {
		t.Fatalf("fan rpm = %v, want 1500 (binary)", m.Fan.RPM)
	}
	if m.Fan.MaxRPM == nil || *m.Fan.MaxRPM != 3000 {
		t.Fatalf("fan max = %v, want 3000 (hwmon)", m.Fan.MaxRPM)
	}
	if m.Fan.PWM == nil || *m.Fan.PWM < 50.1 || *m.Fan.PWM > 50.2 {
		t.Fatalf("fan pwm = %v, want ~50.2 (hwmon fallback, v1.3 has no fan_pwm)", m.Fan.PWM)
	}
}

// A device with nothing readable produces an all-nil sample, not an error.
func TestSampleDeviceEmptyIsHonest(t *testing.T) {
	dir := t.TempDir()

	m := SampleDevice(dev(dir), 7, 0, 1.0)

	if m.Sequence != 7 {
		t.Fatalf("sequence = %d, want 7 (pass-through)", m.Sequence)
	}
	if m.MetricsABI != nil || m.SCLK.Current != nil || m.GpuBusyPercent != nil {
		t.Fatal("empty device must be all-nil, never zero")
	}
	if m.SCLK.Source != "none" {
		t.Fatalf("sclk source = %q, want none", m.SCLK.Source)
	}
	if m.Power.Source != "hwmon" {
		t.Fatalf("power source = %q, want hwmon", m.Power.Source)
	}
	if len(m.Temperatures) != 0 {
		t.Fatalf("temps = %v, want none", m.Temperatures)
	}
}
