package ui

import (
	"fmt"
	"strings"

	"github.com/gputop/gputop-go/internal/model"
)

// NA is how an absent reading renders: never 0, never guessed.
const NA = "N/A"

// Bar glyphs, matching the block characters the graphs use so the panels
// and the braille plots read as one visual system.
const (
	BarFilled = "█"
	BarEmpty  = "░"
)

// Fmt formats an optional number, rendering nil as N/A.
//
// This is the single choke point for the "unavailable is not zero" rule.
func Fmt(value *float64, suffix string, digits int) string {
	if value == nil {
		return NA
	}
	if digits == 0 {
		return fmt.Sprintf("%d%s", int(*value), suffix)
	}
	return fmt.Sprintf("%.*f%s", digits, *value, suffix)
}

// FmtTemp formats a Celsius reading with a degree sign.
func FmtTemp(value *model.Celsius) string {
	if value == nil {
		return NA
	}
	return fmt.Sprintf("%.0f°C", *value)
}

// FmtDuration formats an elapsed duration compactly: 1h 02m, 9s, 3d 07h.
func FmtDuration(seconds *float64) string {
	if seconds == nil {
		return NA
	}
	total := int(max(0.0, *seconds))
	days, rest := total/86400, total%86400
	hours, rest := rest/3600, rest%3600
	minutes, secs := rest/60, rest%60
	switch {
	case days > 0:
		return fmt.Sprintf("%dd %02dh", days, hours)
	case hours > 0:
		return fmt.Sprintf("%dh %02dm", hours, minutes)
	case minutes > 0:
		return fmt.Sprintf("%dm %02ds", minutes, secs)
	default:
		return fmt.Sprintf("%ds", secs)
	}
}

// FmtBytes formats a byte count with binary units (free/btop convention).
func FmtBytes(value *model.Bytes) string {
	if value == nil {
		return NA
	}
	size := float64(*value)
	for _, unit := range []string{"B", "KiB", "MiB", "GiB", "TiB"} {
		// The last unit is the escape hatch: a petabyte-scale value must
		// still land somewhere instead of falling out of the bottom.
		if absFloat(size) < 1024.0 || unit == "TiB" {
			if unit == "B" {
				return fmt.Sprintf("%.0f %s", size, unit)
			}
			return fmt.Sprintf("%.1f %s", size, unit)
		}
		size /= 1024.0
	}
	return fmt.Sprintf("%.1f TiB", size) // unreachable, kept honest
}

// FmtPercent formats a 0-100 percentage.
func FmtPercent(value *model.Percent, digits int) string {
	if value == nil {
		return NA
	}
	return fmt.Sprintf("%.*f%%", digits, *value)
}

// Bar renders a proportional bar of exactly width characters, or N/A when
// the fraction is unknown.
func Bar(fraction *float64, width int) string {
	if fraction == nil {
		return NA
	}
	clamped := min(100.0, max(0.0, *fraction))
	count := int(clamped/100.0*float64(width) + 0.5)
	return strings.Repeat(BarFilled, count) + strings.Repeat(BarEmpty, width-count)
}

func absFloat(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}
