package readers

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// TempFilePattern matches tempN_input files.
var TempFilePattern = regexp.MustCompile(`^temp(\d+)_input$`)

// LabelMap maps hwmon label substrings to canonical names, longest first.
var LabelMap = [][2]string{
	{"junction", "junction"},
	{"hotspot", "junction"},
	{"vram", "mem"},
	{"edge", "edge"},
	{"mem", "mem"},
	{"soc", "soc"},
	{"core", "core"},
	{"vr", "vr"},
	{"l3", "l3"},
}

// TempOrder is the presentation order for sensors.
var TempOrder = []string{"edge", "junction", "mem", "soc", "core", "l3"}

// CanonicalLabel maps an hwmon label onto a canonical name.
func CanonicalLabel(label *string, index int) string {
	if label == nil || strings.TrimSpace(*label) == "" {
		return "temp" + itoaDec(index)
	}
	cleaned := strings.ToLower(strings.TrimSpace(*label))
	for _, pair := range LabelMap {
		if strings.Contains(cleaned, pair[0]) {
			return pair[1]
		}
	}
	return cleaned
}

func itoaDec(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

func sensorRank(name string) (int, int) {
	for i, n := range TempOrder {
		if n == name {
			return 0, i
		}
	}
	return 1, 0
}

// SortedTemperatureNames returns sensor names in presentation order.
func SortedTemperatureNames(names []string) []string {
	out := append([]string{}, names...)
	sort.Slice(out, func(i, j int) bool {
		ai, bi := sensorRank(out[i])
		aj, bj := sensorRank(out[j])
		if ai != aj {
			return ai < aj
		}
		return bi < bj
	})
	return out
}

// ReadTemperatures reads every tempN_input in a hwmon device.
func ReadTemperatures(hwmonDir string) map[string]float64 {
	out := map[string]float64{}
	if hwmonDir == "" {
		return out
	}
	entries, err := os.ReadDir(hwmonDir)
	if err != nil {
		return out
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)
	for _, name := range names {
		m := TempFilePattern.FindStringSubmatch(name)
		if m == nil {
			continue
		}
		var index int
		for _, c := range m[1] {
			index = index*10 + int(c-'0')
		}
		text, ok := ReadText(filepath.Join(hwmonDir, name), ValueLimit)
		if !ok {
			continue
		}
		value, ok := ParseMillidegrees(text)
		if !ok {
			continue
		}
		var labelText *string
		if lt, ok := ReadText(filepath.Join(hwmonDir, "temp"+itoaDec(index)+"_label"), ValueLimit); ok {
			labelText = &lt
		}
		label := CanonicalLabel(labelText, index)
		if _, dup := out[label]; !dup {
			out[label] = value
		}
	}
	return out
}

// ReadPower reads instantaneous power draw and the enforced cap in watts.
func ReadPower(hwmonDir string) (*float64, *float64) {
	if hwmonDir == "" {
		return nil, nil
	}
	var draw *float64
	if text, ok := ReadText(filepath.Join(hwmonDir, "power1_average"), ValueLimit); ok {
		if v, ok := ParsePowerMicrowatts(text); ok {
			draw = &v
		}
	}
	if draw == nil {
		if text, ok := ReadText(filepath.Join(hwmonDir, "power1_input"), ValueLimit); ok {
			if v, ok := ParsePowerMicrowatts(text); ok {
				draw = &v
			}
		}
	}
	var cap *float64
	if text, ok := ReadText(filepath.Join(hwmonDir, "power1_cap"), ValueLimit); ok {
		if v, ok := ParsePowerMicrowatts(text); ok {
			cap = &v
		}
	}
	if cap == nil {
		if text, ok := ReadText(filepath.Join(hwmonDir, "power1_cap_max"), ValueLimit); ok {
			if v, ok := ParsePowerMicrowatts(text); ok {
				cap = &v
			}
		}
	}
	return draw, cap
}

// ReadFan reads (rpm, maxRPM, pwmPercent) from a hwmon device.
func ReadFan(hwmonDir string) (*int, *int, *float64) {
	if hwmonDir == "" {
		return nil, nil, nil
	}
	var rpm, maxRPM *int
	var pwm *float64
	if v, ok := ReadInt(filepath.Join(hwmonDir, "fan1_input")); ok {
		rpm = &v
	}
	if v, ok := ReadInt(filepath.Join(hwmonDir, "fan1_max")); ok {
		maxRPM = &v
	}
	if v, ok := ReadInt(filepath.Join(hwmonDir, "pwm1")); ok {
		p := float64(v) / 255.0 * 100.0
		pwm = &p
	}
	return rpm, maxRPM, pwm
}
