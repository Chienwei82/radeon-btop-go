package readers

import (
	"encoding/binary"
)

// HeaderSize is the 4-byte <HBB header prefixing every metric table.
const HeaderSize = 4

// Sentinels for unsupported fields (driver memset 0xFF over unfilled struct).
const (
	SentinelU16 = 0xFFFF
	SentinelU32 = 0xFFFFFFFF
	SentinelU64 = 0xFFFFFFFFFFFFFFFF
)

// Plausibility bounds per field family.
const (
	MaxClockMHz  = 10_000
	MaxTempC     = 150
	MaxPowerW    = 10_000
	MinVoltageMv = 50
	MaxVoltageMv = 2_000
)

// fieldSpec is one named field in a metric table layout.
type fieldSpec struct {
	name  string
	code  byte // 'B' u8, 'H' u16, 'I' u32, 'Q' u64
	count int
}

func f(name string, code byte) fieldSpec         { return fieldSpec{name, code, 1} }
func fa(name string, code byte, n int) fieldSpec { return fieldSpec{name, code, n} }

var fieldSize = map[byte]int{'B': 1, 'H': 2, 'I': 4, 'Q': 8}

// metricsABI is one entry in the ABI registry.
type metricsABI struct {
	fmtRev     int
	contentRev int
	fields     []fieldSpec
	offsets    map[string]int
	bodySize   int
}

// Label returns the human readable ABI label, e.g. "v1.3".
func (a *metricsABI) Label() string {
	return "v" + digit(a.fmtRev) + "." + digit(a.contentRev)
}

func digit(n int) string {
	if n >= 0 && n < 10 {
		return string(rune('0' + n))
	}
	return "?"
}

var v1Temps = []fieldSpec{
	f("temperature_edge", 'H'),
	f("temperature_hotspot", 'H'),
	f("temperature_mem", 'H'),
	f("temperature_vrgfx", 'H'),
	f("temperature_vrsoc", 'H'),
	f("temperature_vrmem", 'H'),
}

var v1Activities = []fieldSpec{
	f("average_gfx_activity", 'H'),
	f("average_umc_activity", 'H'),
	f("average_mm_activity", 'H'),
}

var v1ClockNames = []string{"gfxclk", "socclk", "uclk", "vclk0", "dclk0", "vclk1", "dclk1"}

// align computes C-style offsets for fields starting after the header.
func align(fields []fieldSpec) (map[string]int, int) {
	offsets := make(map[string]int, len(fields))
	offset := HeaderSize
	for _, spec := range fields {
		size := fieldSize[spec.code]
		if rem := offset % size; rem != 0 {
			offset += size - rem
		}
		offsets[spec.name] = offset
		offset += size * spec.count
	}
	return offsets, offset
}

func buildABI(fmtRev, contentRev int, fields []fieldSpec, sizeof int) *metricsABI {
	offsets, end := align(fields)
	if end > sizeof {
		panic("metricsbin layout overruns kernel sizeof")
	}
	return &metricsABI{fmtRev, contentRev, fields, offsets, sizeof}
}

func buildV10() []fieldSpec {
	fields := []fieldSpec{f("system_clock_counter", 'Q')}
	fields = append(fields, v1Temps...)
	fields = append(fields, v1Activities...)
	fields = append(fields, f("average_socket_power", 'H'), f("energy_accumulator", 'I'))
	for _, n := range v1ClockNames {
		fields = append(fields, f("average_"+n+"_frequency", 'H'))
	}
	for _, n := range v1ClockNames {
		fields = append(fields, f("current_"+n, 'H'))
	}
	fields = append(fields,
		f("throttle_status", 'I'),
		f("current_fan_speed", 'H'),
		f("pcie_link_width", 'B'),
		f("pcie_link_speed", 'B'),
	)
	return fields
}

func buildV11() []fieldSpec {
	fields := append([]fieldSpec{}, v1Temps...)
	fields = append(fields, v1Activities...)
	fields = append(fields,
		f("average_socket_power", 'H'),
		f("energy_accumulator", 'Q'),
		f("system_clock_counter", 'Q'),
	)
	for _, n := range v1ClockNames {
		fields = append(fields, f("average_"+n+"_frequency", 'H'))
	}
	for _, n := range v1ClockNames {
		fields = append(fields, f("current_"+n, 'H'))
	}
	fields = append(fields,
		f("throttle_status", 'I'),
		f("current_fan_speed", 'H'),
		f("pcie_link_width", 'H'),
		f("pcie_link_speed", 'H'),
		f("padding", 'H'),
		f("gfx_activity_acc", 'I'),
		f("mem_activity_acc", 'I'),
		fa("temperature_hbm", 'H', 4),
	)
	return fields
}

func buildV20() []fieldSpec {
	return []fieldSpec{
		f("system_clock_counter", 'Q'),
		f("temperature_gfx", 'H'),
		f("temperature_soc", 'H'),
		fa("temperature_core", 'H', 8),
		fa("temperature_l3", 'H', 2),
		f("average_gfx_activity", 'H'),
		f("average_mm_activity", 'H'),
		f("average_socket_power", 'H'),
		f("average_cpu_power", 'H'),
		f("average_soc_power", 'H'),
		f("average_gfx_power", 'H'),
		fa("average_core_power", 'H', 8),
		f("average_gfxclk_frequency", 'H'),
		f("average_socclk_frequency", 'H'),
		f("average_uclk_frequency", 'H'),
		f("average_fclk_frequency", 'H'),
		f("average_vclk_frequency", 'H'),
		f("average_dclk_frequency", 'H'),
		f("current_gfxclk", 'H'),
		f("current_socclk", 'H'),
		f("current_uclk", 'H'),
		f("current_fclk", 'H'),
		f("current_vclk", 'H'),
		f("current_dclk", 'H'),
		fa("current_coreclk", 'H', 8),
		fa("current_l3clk", 'H', 2),
		f("throttle_status", 'I'),
		f("fan_pwm", 'H'),
		f("padding", 'H'),
	}
}

func buildV21() []fieldSpec {
	return []fieldSpec{
		f("temperature_gfx", 'H'),
		f("temperature_soc", 'H'),
		fa("temperature_core", 'H', 8),
		fa("temperature_l3", 'H', 2),
		f("average_gfx_activity", 'H'),
		f("average_mm_activity", 'H'),
		f("system_clock_counter", 'Q'),
		f("average_socket_power", 'H'),
		f("average_cpu_power", 'H'),
		f("average_soc_power", 'H'),
		f("average_gfx_power", 'H'),
		fa("average_core_power", 'H', 8),
		f("average_gfxclk_frequency", 'H'),
		f("average_socclk_frequency", 'H'),
		f("average_uclk_frequency", 'H'),
		f("average_fclk_frequency", 'H'),
		f("average_vclk_frequency", 'H'),
		f("average_dclk_frequency", 'H'),
		f("current_gfxclk", 'H'),
		f("current_socclk", 'H'),
		f("current_uclk", 'H'),
		f("current_fclk", 'H'),
		f("current_vclk", 'H'),
		f("current_dclk", 'H'),
		fa("current_coreclk", 'H', 8),
		fa("current_l3clk", 'H', 2),
		f("throttle_status", 'I'),
		f("fan_pwm", 'H'),
		f("padding", 'H'),
	}
}

// ABIRegistry holds every layout gputop understands.
var ABIRegistry = buildRegistry()

func buildRegistry() map[[2]int]*metricsABI {
	v10 := buildV10()
	v11 := buildV11()
	v12 := append(append([]fieldSpec{}, v11...), f("firmware_timestamp", 'Q'))
	v13 := append(append([]fieldSpec{}, v12...),
		f("voltage_soc", 'H'),
		f("voltage_gfx", 'H'),
		f("voltage_mem", 'H'),
		f("padding1", 'H'),
		f("indep_throttle_status", 'Q'),
	)
	v20 := buildV20()
	v21 := buildV21()
	v22 := append(append([]fieldSpec{}, v21...), f("indep_throttle_status", 'Q'))
	_ = binary.LittleEndian
	return map[[2]int]*metricsABI{
		{1, 0}: buildABI(1, 0, v10, 80),
		{1, 1}: buildABI(1, 1, v11, 96),
		{1, 2}: buildABI(1, 2, v12, 104),
		{1, 3}: buildABI(1, 3, v13, 120),
		{2, 0}: buildABI(2, 0, v20, 120),
		{2, 1}: buildABI(2, 1, v21, 120),
		{2, 2}: buildABI(2, 2, v22, 128),
	}
}

var tempOrder = []string{"edge", "junction", "mem", "soc", "core", "l3"}

var v1TempFields = [][2]string{
	{"temperature_edge", "edge"},
	{"temperature_hotspot", "junction"},
	{"temperature_mem", "mem"},
}

var v2TempFields = [][2]string{
	{"temperature_gfx", "edge"},
	{"temperature_soc", "soc"},
	{"temperature_core", "core"},
	{"temperature_l3", "l3"},
}
