package readers

import (
	"encoding/binary"
	"testing"

	"github.com/gputop/gputop-go/internal/model"
)

// buildTable packs a metric table for one ABI (test helper, mirrors Python).
func buildTable(t *testing.T, abi *metricsABI, values map[string]uint64, structureSize int, trailing int) []byte {
	t.Helper()
	raw := make([]byte, abi.bodySize+trailing)
	for _, spec := range abi.fields {
		v := values[spec.name]
		off := abi.offsets[spec.name]
		size := fieldSize[spec.code]
		tmp := make([]byte, 8)
		switch spec.code {
		case 'B':
			raw[off] = byte(v)
		case 'H':
			binary.LittleEndian.PutUint16(tmp, uint16(v))
			copy(raw[off:off+size], tmp[:size])
		case 'I':
			binary.LittleEndian.PutUint32(tmp, uint32(v))
			copy(raw[off:off+size], tmp[:size])
		default:
			binary.LittleEndian.PutUint64(tmp, v)
			copy(raw[off:off+size], tmp[:size])
		}
	}
	declared := abi.bodySize + trailing
	if structureSize >= 0 {
		declared = structureSize
	}
	binary.LittleEndian.PutUint16(raw[0:2], uint16(declared))
	raw[2] = byte(abi.fmtRev)
	raw[3] = byte(abi.contentRev)
	return raw
}

func TestEveryABIMatchesKernelSizeof(t *testing.T) {
	want := map[[2]int]int{{1, 0}: 80, {1, 1}: 96, {1, 2}: 104, {1, 3}: 120, {2, 0}: 120, {2, 1}: 120, {2, 2}: 128}
	for rev, size := range want {
		if ABIRegistry[rev].bodySize != size {
			t.Fatalf("ABI %v size = %d, want %d", rev, ABIRegistry[rev].bodySize, size)
		}
	}
}

func TestV13OffsetsMatchHardware(t *testing.T) {
	want := map[string]int{
		"temperature_edge": 4, "temperature_hotspot": 6, "temperature_mem": 8,
		"average_gfx_activity": 16, "average_socket_power": 22, "energy_accumulator": 24,
		"system_clock_counter": 32, "current_gfxclk": 54, "current_uclk": 58,
		"throttle_status": 68, "current_fan_speed": 72,
		"pcie_link_width": 74, "pcie_link_speed": 76,
		"temperature_hbm": 88, "firmware_timestamp": 96,
		"voltage_soc": 104, "voltage_gfx": 106, "voltage_mem": 108,
		"indep_throttle_status": 112,
	}
	abi := ABIRegistry[[2]int{1, 3}]
	for name, off := range want {
		if abi.offsets[name] != off {
			t.Fatalf("%s at %d, want %d", name, abi.offsets[name], off)
		}
	}
}

func TestV21KernelOffsets(t *testing.T) {
	want := map[string]int{
		"temperature_gfx": 4, "temperature_soc": 6, "temperature_core": 8,
		"temperature_l3": 24, "average_gfx_activity": 28, "average_mm_activity": 30,
		"system_clock_counter": 32, "average_socket_power": 40, "average_core_power": 48,
		"average_gfxclk_frequency": 64, "current_gfxclk": 76, "current_coreclk": 88,
		"current_l3clk": 104, "throttle_status": 108, "fan_pwm": 112,
	}
	abi := ABIRegistry[[2]int{2, 1}]
	for name, off := range want {
		if abi.offsets[name] != off {
			t.Fatalf("%s at %d, want %d", name, abi.offsets[name], off)
		}
	}
}

func TestUnknownRevisionReturnsNil(t *testing.T) {
	blob := make([]byte, 120)
	binary.LittleEndian.PutUint16(blob[0:2], 120)
	blob[2], blob[3] = 3, 0
	if got := (GpuMetricsParser{}).Parse(blob); got != nil {
		t.Fatal("3.x must fall back to sysfs (nil), never guessed")
	}
	if got := (GpuMetricsParser{}).Parse([]byte{1, 2}); got != nil {
		t.Fatal("truncated header must be nil")
	}
}

func TestSentinelsDecodeToNil(t *testing.T) {
	abi := ABIRegistry[[2]int{1, 3}]
	blob := buildTable(t, abi, map[string]uint64{
		"average_umc_activity": SentinelU16,
		"gfx_activity_acc":     SentinelU32,
		"firmware_timestamp":   SentinelU64,
	}, -1, 0)
	parsed := (GpuMetricsParser{}).Parse(blob)
	if parsed == nil {
		t.Fatal("parse returned nil")
	}
	if parsed.UmcActivity != nil {
		t.Fatal("sentinel activity must be nil")
	}
}

func TestZeroClockKeptBecauseIdleIsReal(t *testing.T) {
	abi := ABIRegistry[[2]int{1, 3}]
	blob := buildTable(t, abi, map[string]uint64{"current_gfxclk": 0, "average_gfx_activity": 0}, -1, 0)
	parsed := (GpuMetricsParser{}).Parse(blob)
	if parsed == nil {
		t.Fatal("parse returned nil")
	}
	if parsed.SclkMHz == nil || *parsed.SclkMHz != 0 {
		t.Fatal("zero clock is idle, not absent")
	}
	if parsed.GpuActivity == nil || *parsed.GpuActivity != 0 {
		t.Fatal("zero activity is real, not absent")
	}
}

func TestImplausibleClockRejected(t *testing.T) {
	abi := ABIRegistry[[2]int{1, 3}]
	blob := buildTable(t, abi, map[string]uint64{"current_gfxclk": 60000}, -1, 0)
	parsed := (GpuMetricsParser{}).Parse(blob)
	if parsed == nil {
		t.Fatal("parse returned nil")
	}
	if parsed.SclkMHz != nil {
		t.Fatalf("60000 MHz must be nil, got %d", *parsed.SclkMHz)
	}
}

func TestAbsentVoltageRailRejected(t *testing.T) {
	abi := ABIRegistry[[2]int{1, 3}]
	blob := buildTable(t, abi, map[string]uint64{"voltage_soc": 918, "voltage_gfx": 0, "voltage_mem": 900}, -1, 0)
	parsed := (GpuMetricsParser{}).Parse(blob)
	if parsed == nil {
		t.Fatal("parse returned nil")
	}
	if len(parsed.VoltagesMv) != 2 {
		t.Fatalf("voltages = %v, want soc+mem only", parsed.VoltagesMv)
	}
}

func TestRealHardwareBlobDecodes(t *testing.T) {
	abi := ABIRegistry[[2]int{1, 3}]
	blob := buildTable(t, abi, map[string]uint64{
		"temperature_edge": 49, "temperature_hotspot": 56, "temperature_mem": 52,
		"average_gfx_activity": 8, "average_socket_power": 33,
		"current_gfxclk": 500, "current_uclk": 1000,
		"pcie_link_width": 16, "pcie_link_speed": 80,
		"voltage_soc": 918, "voltage_gfx": 768, "voltage_mem": 900,
	}, -1, 0)
	parsed := (GpuMetricsParser{}).Parse(blob)
	if parsed == nil {
		t.Fatal("parse returned nil")
	}
	if len(parsed.Temperatures) != 3 {
		t.Fatalf("temps = %v", parsed.Temperatures)
	}
	if *parsed.Temperatures[0].Celsius != 49 || *parsed.Temperatures[1].Celsius != 56 {
		t.Fatalf("temps = %v", parsed.Temperatures)
	}
	if parsed.PCIE == nil || parsed.PCIE.Describe() != "Gen3 x16" {
		t.Fatalf("pcie = %v", parsed.PCIE)
	}
	if len(parsed.VoltagesMv) != 3 || parsed.VoltagesMv[0] != (model.VoltageReading{Name: "soc", MV: 918}) {
		t.Fatalf("voltages = %v", parsed.VoltagesMv)
	}
	if parsed.StructureSize != 120 {
		t.Fatalf("size = %d", parsed.StructureSize)
	}
}

func TestTruncatedTableDecodesPrefix(t *testing.T) {
	abi := ABIRegistry[[2]int{1, 3}]
	full := buildTable(t, abi, map[string]uint64{"temperature_edge": 44, "average_gfx_activity": 21}, 60, 0)
	blob := full[:60]
	parsed := (GpuMetricsParser{}).Parse(blob)
	if parsed == nil {
		t.Fatal("parse returned nil")
	}
	if parsed.GpuActivity == nil || *parsed.GpuActivity != 21 {
		t.Fatal("prefix fields must decode")
	}
	if parsed.FanRPM != nil || parsed.Throttle != nil || len(parsed.VoltagesMv) != 0 {
		t.Fatal("fields past trim must be absent, not zero")
	}
}

func TestTrailingBytesIgnored(t *testing.T) {
	abi := ABIRegistry[[2]int{1, 3}]
	blob := buildTable(t, abi, map[string]uint64{"current_gfxclk": 900}, -1, 32)
	parsed := (GpuMetricsParser{}).Parse(blob)
	if parsed == nil || parsed.SclkMHz == nil || *parsed.SclkMHz != 900 {
		t.Fatalf("sclk = %v", parsed)
	}
}
