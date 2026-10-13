package readers

import (
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// DPMRow matches one row of a pp_dpm_* table: "<level>: <N> Mhz [*]".
var DPMRow = regexp.MustCompile(`^(\S+):\s*(\d+)\s*([KMUG]?[Hh]z)\s*(\*)?\s*$`)

// LinkSpeedRow matches "16.0 GT/s" inside current_link_speed.
var LinkSpeedRow = regexp.MustCompile(`(\d+(?:\.\d+)?)\s*GT/s`)

// ParseDPMTable parses a pp_dpm_* table into (currentMHz, maxMHz).
func ParseDPMTable(text string) (*int, *int) {
	if text == "" {
		return nil, nil
	}
	var first, current, maximum *int
	for _, line := range strings.Split(text, "\n") {
		m := DPMRow.FindStringSubmatch(strings.TrimSpace(line))
		if m == nil {
			continue
		}
		mhz, err := strconv.Atoi(m[2])
		if err != nil {
			continue
		}
		if first == nil {
			v := mhz
			first = &v
		}
		if m[4] != "" && current == nil {
			v := mhz
			current = &v
		}
		if maximum == nil || mhz > *maximum {
			v := mhz
			maximum = &v
		}
	}
	if current == nil {
		current = first
	}
	return current, maximum
}

// ReadClock reads one clock using the full fallback chain.
// Returns (currentMHz, confidence): 2 direct attribute, 1 DPM table, 0 none.
func ReadClock(deviceDir, attribute, dpmFile string) (*int, int) {
	if text, ok := ReadText(filepath.Join(deviceDir, attribute), ValueLimit); ok {
		if current, ok := ParseFirstInt(text); ok {
			var maximum *int
			if mx, ok := ReadText(filepath.Join(deviceDir, strings.Replace(attribute, "current_", "max_", 1)), ValueLimit); ok {
				if v, ok := ParseFirstInt(mx); ok {
					maximum = &v
				}
			}
			_ = maximum
			return &current, 2
		}
	}
	// Fall through to the DPM table (current + ceiling from rows).
	if text, ok := ReadText(filepath.Join(deviceDir, dpmFile), TableLimit); ok {
		current, _ := ParseDPMTable(text)
		if current != nil {
			return current, 1
		}
	}
	return nil, 0
}

// ReadClockPair reads (current, maximum) with the confidence chain.
func ReadClockPair(deviceDir, attribute, dpmFile string) (*int, *int, int) {
	if text, ok := ReadText(filepath.Join(deviceDir, attribute), ValueLimit); ok {
		if current, ok := ParseFirstInt(text); ok {
			maxAttr := strings.Replace(attribute, "current_", "max_", 1)
			maximum, _ := func() (*int, bool) {
				if t, ok := ReadText(filepath.Join(deviceDir, maxAttr), ValueLimit); ok {
					if v, ok := ParseFirstInt(t); ok {
						return &v, true
					}
				}
				return nil, false
			}()
			if maximum == nil {
				if t, ok := ReadText(filepath.Join(deviceDir, dpmFile), TableLimit); ok {
					_, maximum = ParseDPMTable(t)
				}
			}
			return &current, maximum, 2
		}
	}
	if text, ok := ReadText(filepath.Join(deviceDir, dpmFile), TableLimit); ok {
		current, maximum := ParseDPMTable(text)
		if current != nil {
			return current, maximum, 1
		}
	}
	return nil, nil, 0
}

// ReadClockMax reads a clock ceiling via attribute then highest DPM row.
func ReadClockMax(deviceDir, attribute, dpmFile string) *int {
	if text, ok := ReadText(filepath.Join(deviceDir, attribute), ValueLimit); ok {
		if v, ok := ParseFirstInt(text); ok {
			return &v
		}
	}
	if text, ok := ReadText(filepath.Join(deviceDir, dpmFile), TableLimit); ok {
		_, maximum := ParseDPMTable(text)
		return maximum
	}
	return nil
}

// ReadPerformanceLevel reads the active DPM performance level.
func ReadPerformanceLevel(deviceDir string) *string {
	for _, attr := range []string{"power_dpm_force_performance_level", "pp_dpm_performance_level"} {
		if v, ok := ReadText(filepath.Join(deviceDir, attr), ValueLimit); ok && v != "" {
			return &v
		}
	}
	return nil
}

// ParsePowerProfile extracts the active SCPP profile name.
func ParsePowerProfile(text string) *string {
	if text == "" {
		return nil
	}
	var fallback *string
	for _, line := range strings.Split(text, "\n") {
		m := ProfileRow.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		name := strings.TrimSpace(m[2])
		if fallback == nil {
			v := name
			fallback = &v
		}
		if m[3] != "" {
			v := name
			return &v
		}
	}
	return fallback
}

// ReadPowerProfile reads the active SCPP power profile name.
func ReadPowerProfile(deviceDir string) *string {
	if text, ok := ReadText(filepath.Join(deviceDir, "pp_power_profile_mode"), TableLimit); ok {
		return ParsePowerProfile(text)
	}
	return nil
}

// ParseLinkSpeed parses current_link_speed into 0.1 GT/s units.
func ParseLinkSpeed(text string) *int {
	if text == "" {
		return nil
	}
	m := LinkSpeedRow.FindStringSubmatch(text)
	if m == nil {
		return nil
	}
	rate, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		return nil
	}
	v := int(rate*10 + 0.5)
	return &v
}

// ReadLink reads the negotiated (width, speed) of the PCIe link.
func ReadLink(deviceDir string) (*int, *int) {
	var width, speed *int
	if text, ok := ReadText(filepath.Join(deviceDir, "current_link_width"), ValueLimit); ok {
		if v, ok := ParseFirstInt(text); ok {
			width = &v
		}
	}
	if text, ok := ReadText(filepath.Join(deviceDir, "current_link_speed"), ValueLimit); ok {
		speed = ParseLinkSpeed(text)
	}
	return width, speed
}

// ChooseClock picks a clock value and labels its source.
func ChooseClock(sysfsValue *int, binaryValue *int, confidence int) (*int, string) {
	if binaryValue != nil {
		return binaryValue, "gpu_metrics"
	}
	if sysfsValue != nil && confidence == 2 {
		return sysfsValue, "sysfs"
	}
	if sysfsValue != nil {
		return sysfsValue, "dpm"
	}
	return nil, "none"
}
