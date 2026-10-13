package readers

import (
	"encoding/binary"

	"github.com/gputop/gputop-go/internal/model"
)

// ParsedGpuMetrics is the decoded table on canonical metric names.
type ParsedGpuMetrics struct {
	ABILabel        string
	FormatRevision  int
	ContentRevision int
	StructureSize   int
	GpuActivity     *float64
	UmcActivity     *float64
	MmActivity      *float64
	Temperatures    []model.Temperature
	PowerDrawW      *float64
	EnergyMJ        *int64
	SclkMHz         *int
	SocclkMHz       *int
	MclkMHz         *int
	VclkMHz         *int
	DclkMHz         *int
	FclkMHz         *int
	FanRPM          *int
	FanPWMPercent   *float64
	Throttle        *model.ThrottleInfo
	PCIE            *model.PcieLink
	VoltagesMv      []model.VoltageReading
}

func readRaw(blob []byte, abi *metricsABI, name string, present int) (uint64, bool) {
	off, ok := abi.offsets[name]
	if !ok || off >= present {
		return 0, false
	}
	var code byte
	found := false
	for i := range abi.fields {
		if abi.fields[i].name == name {
			code = abi.fields[i].code
			found = true
			break
		}
	}
	if !found {
		return 0, false
	}
	size := fieldSize[code]
	if off+size > len(blob) {
		return 0, false
	}
	switch code {
	case 'B':
		return uint64(blob[off]), true
	case 'H':
		return uint64(binary.LittleEndian.Uint16(blob[off:])), true
	case 'I':
		return uint64(binary.LittleEndian.Uint32(blob[off:])), true
	default:
		return binary.LittleEndian.Uint64(blob[off:]), true
	}
}

func getU16(blob []byte, abi *metricsABI, name string, present int) *int {
	v, ok := readRaw(blob, abi, name, present)
	if !ok || v == SentinelU16 {
		return nil
	}
	out := int(v)
	return &out
}

func getU32(blob []byte, abi *metricsABI, name string, present int) *int {
	v, ok := readRaw(blob, abi, name, present)
	if !ok || v == SentinelU32 {
		return nil
	}
	out := int(v)
	return &out
}

func getU64(blob []byte, abi *metricsABI, name string, present int) *uint64 {
	v, ok := readRaw(blob, abi, name, present)
	if !ok || v == SentinelU64 {
		return nil
	}
	return &v
}

func keepClock(v *int) *int {
	if v == nil || *v < 0 || *v > MaxClockMHz {
		return nil
	}
	return v
}

func keepTemp(v *int, label string) *model.Temperature {
	if v == nil || *v < 0 || *v > MaxTempC {
		return nil
	}
	c := float64(*v)
	return &model.Temperature{Label: label, Celsius: &c, Source: "gpu_metrics"}
}

func keepVolt(v *int, name string) *model.VoltageReading {
	if v == nil || *v < MinVoltageMv || *v > MaxVoltageMv {
		return nil
	}
	return &model.VoltageReading{Name: name, MV: *v}
}

func keepPct(v *int) *float64 {
	if v == nil || *v < 0 || *v > 100 {
		return nil
	}
	f := float64(*v)
	return &f
}

func keepPower(v *int) *float64 {
	if v == nil || *v < 0 || *v > MaxPowerW {
		return nil
	}
	f := float64(*v)
	return &f
}

// GpuMetricsParser decodes the binary gpu_metrics file. Stateless.
type GpuMetricsParser struct{}

// ParseFile parses a gpu_metrics file path.
func (GpuMetricsParser) ParseFile(path string) *ParsedGpuMetrics {
	blob, ok := ReadBytes(path, TableLimit)
	if !ok {
		return nil
	}
	return (GpuMetricsParser{}).Parse(blob)
}

// Parse parses a metric table from raw bytes.
func (GpuMetricsParser) Parse(blob []byte) *ParsedGpuMetrics {
	if len(blob) < HeaderSize {
		return nil
	}
	structureSize := int(binary.LittleEndian.Uint16(blob[0:2]))
	fmtRev := int(blob[2])
	contentRev := int(blob[3])
	if HeaderSize >= structureSize || structureSize > len(blob) {
		return nil
	}
	abi, ok := ABIRegistry[[2]int{fmtRev, contentRev}]
	if !ok {
		return nil
	}
	present := len(blob)
	if abi.bodySize > len(blob) {
		present = structureSize
		padded := make([]byte, abi.bodySize)
		copy(padded, blob)
		blob = padded
	}
	return toModel(blob, abi, structureSize, present)
}

func toModel(blob []byte, abi *metricsABI, structureSize, present int) *ParsedGpuMetrics {
	temps := map[string]model.Temperature{}
	allTempFields := append(append([][2]string{}, v1TempFields...), v2TempFields...)
	for _, pair := range allTempFields {
		if t := keepTemp(getU16(blob, abi, pair[0], present), pair[1]); t != nil {
			if _, dup := temps[pair[1]]; !dup {
				temps[pair[1]] = *t
			}
		}
	}
	ordered := []model.Temperature{}
	for _, name := range tempOrder {
		if t, ok := temps[name]; ok {
			ordered = append(ordered, t)
		}
	}
	var pcie *model.PcieLink
	w := getU16(blob, abi, "pcie_link_width", present)
	s := getU16(blob, abi, "pcie_link_speed", present)
	if w != nil && s != nil && *w != 0 && *s != 0 {
		pcie = &model.PcieLink{Width: *w, Speed: *s}
	}
	var throttle *model.ThrottleInfo
	if raw := getU32(blob, abi, "throttle_status", present); raw != nil {
		active := DecodeThrottle(*raw)
		throttle = &model.ThrottleInfo{Raw: *raw, Active: active}
	}
	var volts []model.VoltageReading
	voltFields := [][2]string{{"voltage_soc", "soc"}, {"voltage_gfx", "gfx"}, {"voltage_mem", "mem"}}
	for _, pair := range voltFields {
		if v := keepVolt(getU16(blob, abi, pair[0], present), pair[1]); v != nil {
			volts = append(volts, *v)
		}
	}
	var energy *int64
	for i := range abi.fields {
		if abi.fields[i].name != "energy_accumulator" {
			continue
		}
		if abi.offsets["energy_accumulator"] >= present {
			break
		}
		if abi.fields[i].code == 'I' {
			if v := getU32(blob, abi, "energy_accumulator", present); v != nil {
				e := int64(*v)
				energy = &e
			}
		} else if v := getU64(blob, abi, "energy_accumulator", present); v != nil {
			e := int64(*v)
			energy = &e
		}
		break
	}
	return &ParsedGpuMetrics{
		ABILabel: abi.Label(), FormatRevision: abi.fmtRev, ContentRevision: abi.contentRev,
		StructureSize: structureSize,
		GpuActivity:   keepPct(getU16(blob, abi, "average_gfx_activity", present)),
		UmcActivity:   keepPct(getU16(blob, abi, "average_umc_activity", present)),
		MmActivity:    keepPct(getU16(blob, abi, "average_mm_activity", present)),
		Temperatures:  ordered,
		PowerDrawW:    keepPower(getU16(blob, abi, "average_socket_power", present)),
		EnergyMJ:      energy,
		SclkMHz:       keepClock(getU16(blob, abi, "current_gfxclk", present)),
		SocclkMHz:     keepClock(getU16(blob, abi, "current_socclk", present)),
		MclkMHz:       keepClock(getU16(blob, abi, "current_uclk", present)),
		VclkMHz:       keepClock(getU16(blob, abi, "current_vclk0", present)),
		DclkMHz:       keepClock(getU16(blob, abi, "current_dclk0", present)),
		FclkMHz:       keepClock(getU16(blob, abi, "current_fclk", present)),
		FanRPM:        getU16(blob, abi, "current_fan_speed", present),
		FanPWMPercent: keepPct(getU16(blob, abi, "fan_pwm", present)),
		Throttle:      throttle,
		PCIE:          pcie,
		VoltagesMv:    volts,
	}
}

// UnrecognisedHeader returns (size, format, content) for an unparsed table.
func UnrecognisedHeader(blob []byte) (int, int, int, bool) {
	if len(blob) < HeaderSize {
		return 0, 0, 0, false
	}
	return int(binary.LittleEndian.Uint16(blob[0:2])), int(blob[2]), int(blob[3]), true
}
