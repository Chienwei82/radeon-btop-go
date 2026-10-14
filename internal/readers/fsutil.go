package readers

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// ValueLimit is bytes read from a single-value sysfs attribute.
const ValueLimit = 4096

// TableLimit is bytes read from a multi-row sysfs table.
const TableLimit = 65_536

// ProfileRow matches one row of pp_power_profile_mode: "<id> <NAME>".
// Group 3 is the optional trailing star; the trailing ":" is decoration.
// Verbatim port of Python's PROFILE_ROW.
var ProfileRow = regexp.MustCompile(`^\s*(\d+)\s+([A-Za-z0-9_][A-Za-z0-9_ -]*?)\s*(\*)?:?\s*$`)

// ReadText reads a small text file, returning "" when unreadable.
func ReadText(path string, limit int) (string, bool) {
	if limit <= 0 {
		limit = ValueLimit
	}
	f, err := os.Open(path)
	if err != nil {
		return "", false
	}
	defer func() { _ = f.Close() }()
	buf := make([]byte, limit)
	n, err := f.Read(buf)
	if err != nil && n == 0 {
		return "", false
	}
	return strings.TrimSpace(string(buf[:n])), true
}

// ReadBytes reads up to limit bytes from a binary file.
func ReadBytes(path string, limit int) ([]byte, bool) {
	if limit <= 0 {
		limit = TableLimit
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	if len(data) > limit {
		data = data[:limit]
	}
	return data, true
}

// ReadInt reads a file whose entire content is a single integer.
func ReadInt(path string) (int, bool) {
	text, ok := ReadText(path, ValueLimit)
	if !ok {
		return 0, false
	}
	return ParseInt(text)
}

// ParseInt parses a stripped integer string.
func ParseInt(text string) (int, bool) {
	text = strings.TrimSpace(text)
	if text == "" {
		return 0, false
	}
	v, err := strconv.Atoi(text)
	if err != nil {
		return 0, false
	}
	return v, true
}

// ReadHexInt reads a hexadecimal integer such as 0x1002.
func ReadHexInt(path string) (int, bool) {
	text, ok := ReadText(path, ValueLimit)
	if !ok {
		return 0, false
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return 0, false
	}
	v, err := strconv.ParseInt(text, 16, 64)
	if err != nil {
		// Accept a leading 0x or plain hex alike; ParseInt with base 16
		// rejects "0x" prefix, so strip it first.
		clean := strings.TrimPrefix(strings.TrimPrefix(text, "0x"), "0X")
		v2, err2 := strconv.ParseInt(clean, 16, 64)
		if err2 != nil {
			return 0, false
		}
		return int(v2), true
	}
	return int(v), true
}

var firstIntRe = regexp.MustCompile(`-?\d+`)

// ParseFirstInt returns the first integer appearing in a file.
func ParseFirstInt(text string) (int, bool) {
	if text == "" {
		return 0, false
	}
	m := firstIntRe.FindString(text)
	if m == "" {
		return 0, false
	}
	return ParseInt(m)
}

// ReadFirstInt reads a file and returns the first integer found.
func ReadFirstInt(path string) (int, bool) {
	text, ok := ReadText(path, ValueLimit)
	if !ok {
		return 0, false
	}
	return ParseFirstInt(text)
}

// LinkTarget resolves a symlink without raising.
func LinkTarget(path string) (string, bool) {
	target, err := os.Readlink(path)
	if err != nil {
		return "", false
	}
	return target, true
}

// Resolve resolves a path without raising.
func Resolve(path string) (string, bool) {
	abs, err := filepath.EvalSymlinks(path)
	if err != nil {
		// Non-strict resolution: return the cleaned path on failure,
		// mirroring Path.resolve(strict=False).
		if _, statErr := os.Lstat(path); statErr != nil {
			return "", false
		}
		return filepath.Clean(path), true
	}
	return abs, true
}

// ParsePowerMicrowatts converts a hwmon micro-watt reading into watts.
func ParsePowerMicrowatts(text string) (float64, bool) {
	v, ok := ParseInt(strings.TrimSpace(text))
	if !ok {
		return 0, false
	}
	return float64(v) / 1_000_000.0, true
}

// ParseMillidegrees converts a hwmon milli-degree reading into Celsius.
func ParseMillidegrees(text string) (float64, bool) {
	v, ok := ParseInt(strings.TrimSpace(text))
	if !ok {
		return 0, false
	}
	return float64(v) / 1000.0, true
}

var sizeUnits = map[string]int64{
	"b": 1, "kb": 1000, "mb": 1000 * 1000, "gb": 1000 * 1000 * 1000,
	"kib": 1024, "mib": 1024 * 1024, "gib": 1024 * 1024 * 1024,
	"tib": 1024 * 1024 * 1024 * 1024,
}

// ParseDrmSize parses a DRM fdinfo size value such as "4116 KiB".
func ParseDrmSize(text string) (int64, bool) {
	text = strings.TrimSpace(strings.ReplaceAll(text, "\t", " "))
	if text == "" {
		return 0, false
	}
	parts := strings.Fields(text)
	if len(parts) == 0 {
		return 0, false
	}
	number, ok := ParseInt(parts[0])
	if !ok {
		return 0, false
	}
	unit := "b"
	if len(parts) > 1 {
		unit = strings.ToLower(parts[1])
	}
	factor, ok := sizeUnits[unit]
	if !ok {
		// Accept TB as well for forward compatibility.
		if unit == "tb" {
			factor = 1000 * 1000 * 1000 * 1000
		} else {
			return 0, false
		}
	}
	return int64(number) * factor, true
}
