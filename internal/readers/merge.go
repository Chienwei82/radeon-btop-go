package readers

import (
	"path/filepath"

	"github.com/gputop/gputop-go/internal/model"
)

// SampleDevice reads every source for one device and normalises them into a
// single AmdgpuMetrics, applying the source-precedence chain:
//
//	gpu_metrics ▸ device sysfs ▸ hwmon ▸ DPM tables
//
// It is the Go counterpart of the reference Sampler._sample_device, extracted
// as a pure function of (device, filesystem) so it is testable without a
// running sampler and without a GPU. sequence/timestamp/interval are supplied
// by the caller (the sampler owns them). Every reader it calls is total: a
// missing table is data, never an error, and every absent metric stays nil.
func SampleDevice(device model.AmdgpuDevice, sequence int, timestampNs model.Nanoseconds, intervalS float64) model.AmdgpuMetrics {
	deviceDir := device.DeviceDir
	hwmonDir := ""
	if device.HwmonDir != nil {
		hwmonDir = *device.HwmonDir
	}

	parsed := (GpuMetricsParser{}).ParseFile(filepath.Join(deviceDir, "gpu_metrics"))
	hwmonTemps := ReadTemperatures(hwmonDir)
	hwmonPower, powerCap := ReadPower(hwmonDir)
	hwmonFanRPM, fanMax, hwmonFanPWM := ReadFan(hwmonDir)

	sclk, sclkMax, sclkConf := ReadClockPair(deviceDir, "current_sclk", "pp_dpm_sclk")
	mclk, mclkMax, mclkConf := ReadClockPair(deviceDir, "current_mclk", "pp_dpm_mclk")
	socclk, socclkMax, socclkConf := ReadClockPair(deviceDir, "current_socclk", "pp_dpm_socclk")

	// Current value and ceiling come from one pass over the fallback chain.
	vramTotal := readInt64(filepath.Join(deviceDir, "mem_info_vis_vram_total"))
	if vramTotal == nil {
		vramTotal = readInt64(filepath.Join(deviceDir, "mem_info_vram_total"))
	}
	profiles := ReadPowerProfileTable(deviceDir)

	// Binary values, when the table was parsed at all.
	var (
		binActivity, binUmc         *float64
		binPower, binFanPWM         *float64
		binSclk, binMclk, binSocclk *int
		binFanRPM                   *int
		binEnergy                   *int64
		voltages                    []model.VoltageReading
		throttle                    *model.ThrottleInfo
		abi                         *string
	)
	if parsed != nil {
		binActivity, binUmc = parsed.GpuActivity, parsed.UmcActivity
		binPower, binFanPWM = parsed.PowerDrawW, parsed.FanPWMPercent
		binSclk, binMclk, binSocclk = parsed.SclkMHz, parsed.MclkMHz, parsed.SocclkMHz
		binFanRPM, binEnergy = parsed.FanRPM, parsed.EnergyMJ
		voltages, throttle = parsed.VoltagesMv, parsed.Throttle
		label := parsed.ABILabel
		abi = &label
	}

	// gpu_metrics wins for the current clock; the ceiling stays on the sysfs/DPM
	// chain, never from the binary.
	sclkCur, sclkSource := ChooseClock(sclk, binSclk, sclkConf)
	mclkCur, mclkSource := ChooseClock(mclk, binMclk, mclkConf)
	socclkCur, socclkSource := ChooseClock(socclk, binSocclk, socclkConf)

	powerSource := "hwmon"
	if binPower != nil {
		powerSource = "gpu_metrics"
	}
	powerProfile := profiles.ActiveName()
	if powerProfile == "" {
		if p := ReadPowerProfile(deviceDir); p != nil {
			powerProfile = *p
		}
	}

	return model.AmdgpuMetrics{
		Device:      device,
		Sequence:    sequence,
		TimestampNs: timestampNs,
		IntervalS:   intervalS,
		GpuBusyPercent: firstNotNil(
			binActivity, percentAttribute(deviceDir, "gpu_busy_percent"),
		),
		MemBusyPercent: firstNotNil(
			binUmc, percentAttribute(deviceDir, "mem_busy_percent"),
		),
		VRAM: model.MemoryPool{
			Used:   readInt64(filepath.Join(deviceDir, "mem_info_vis_vram_used")),
			Total:  vramTotal,
			Source: "sysfs",
		},
		GTT: model.MemoryPool{
			Used:   readInt64(filepath.Join(deviceDir, "mem_info_gtt_used")),
			Total:  readInt64(filepath.Join(deviceDir, "mem_info_gtt_total")),
			Source: "sysfs",
		},
		SCLK:         model.Clock{Current: sclkCur, Maximum: sclkMax, Source: sclkSource},
		MCLK:         model.Clock{Current: mclkCur, Maximum: mclkMax, Source: mclkSource},
		SocClk:       model.Clock{Current: socclkCur, Maximum: socclkMax, Source: socclkSource},
		VoltagesMv:   voltages,
		Temperatures: mergeTemperatures(hwmonTemps, parsed),
		Power: model.Power{
			DrawW:   firstNotNil(binPower, hwmonPower),
			CapW:    powerCap,
			EnergyM: binEnergy,
			Source:  powerSource,
		},
		Fan: model.Fan{
			RPM:    firstNotNil(binFanRPM, hwmonFanRPM),
			MaxRPM: fanMax,
			PWM:    firstNotNil(binFanPWM, hwmonFanPWM),
		},
		PerformanceLevel: ReadPerformanceLevel(deviceDir),
		PowerProfile:     ptrIfNotEmpty(powerProfile),
		Throttle:         throttle,
		PCIE:             pcieLink(deviceDir, parsed),
		MetricsABI:       abi,
		ODC:              ReadODCVoltage(deviceDir),
		Profiles:         profiles,
	}
}

// readInt64 reads a single-integer file into a *int64 (the model's Bytes alias).
func readInt64(path string) *int64 {
	v, ok := ReadInt(path)
	if !ok {
		return nil
	}
	b := int64(v)
	return &b
}

// ptrIfNotEmpty returns a pointer to s, or nil when s is empty.
func ptrIfNotEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// firstNotNil returns the first non-nil pointer, mirroring the Python
// _first_not_none: the first source in precedence order that has a value.
func firstNotNil[T any](values ...*T) *T {
	for _, v := range values {
		if v != nil {
			return v
		}
	}
	return nil
}

// percentAttribute reads a 0-100 sysfs activity attribute, or nil when absent
// or out of range (a value outside 0-100 is not a percentage).
func percentAttribute(deviceDir, attribute string) *float64 {
	v, ok := ReadInt(filepath.Join(deviceDir, attribute))
	if !ok || v < 0 || v > 100 {
		return nil
	}
	f := float64(v)
	return &f
}

// pcieLink resolves the PCIe link, preferring the binary source and falling
// back to the current_link_* sysfs attributes.
func pcieLink(deviceDir string, parsed *ParsedGpuMetrics) *model.PcieLink {
	if parsed != nil && parsed.PCIE != nil {
		return parsed.PCIE
	}
	width, speed := ReadLink(deviceDir)
	if width != nil && speed != nil {
		return &model.PcieLink{Width: *width, Speed: *speed}
	}
	return nil
}

// mergeTemperatures merges hwmon and gpu_metrics sensors, preferring the
// binary source where they overlap. hwmon is read first so a sensor only it
// knows about (a VRM rail) survives when gpu_metrics is available.
func mergeTemperatures(hwmon map[string]float64, parsed *ParsedGpuMetrics) []model.Temperature {
	merged := make(map[string]model.Temperature, len(hwmon))
	for name, value := range hwmon {
		c := value
		merged[name] = model.Temperature{Label: name, Celsius: &c, Source: "hwmon"}
	}
	if parsed != nil {
		for _, temp := range parsed.Temperatures {
			merged[temp.Label] = temp
		}
	}
	names := make([]string, 0, len(merged))
	for name := range merged {
		names = append(names, name)
	}
	ordered := SortedTemperatureNames(names)
	out := make([]model.Temperature, 0, len(ordered))
	for _, name := range ordered {
		out = append(out, merged[name])
	}
	return out
}
