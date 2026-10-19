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
// Numbers are grouped in thousands, the way the reference implementation's
// f"{value:,}" does, so a 2 200 MHz clock reads "2,200 MHz" in both.
func Fmt(value *float64, suffix string, digits int) string {
	if value == nil {
		return NA
	}
	if digits == 0 {
		return groupNumber(fmt.Sprintf("%d", int(*value))) + suffix
	}
	return groupNumber(fmt.Sprintf("%.*f", digits, *value)) + suffix
}

// groupNumber groups the integer part of an already formatted number,
// leaving the fractional part alone.
func groupNumber(s string) string {
	if i := strings.IndexByte(s, '.'); i >= 0 {
		return groupThousands(s[:i]) + s[i:]
	}
	return groupThousands(s)
}

// groupThousands inserts the separator every three digits, counting from the
// right and carrying the sign outside the grouping.
func groupThousands(digits string) string {
	sign := ""
	if strings.HasPrefix(digits, "-") || strings.HasPrefix(digits, "+") {
		sign, digits = digits[:1], digits[1:]
	}
	if len(digits) <= 3 {
		return sign + digits
	}
	var b strings.Builder
	b.WriteString(sign)
	lead := len(digits) % 3
	if lead == 0 {
		lead = 3
	}
	b.WriteString(digits[:lead])
	for i := lead; i < len(digits); i += 3 {
		b.WriteByte(',')
		b.WriteString(digits[i : i+3])
	}
	return b.String()
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
				return groupNumber(fmt.Sprintf("%.0f", size)) + " " + unit
			}
			return groupNumber(fmt.Sprintf("%.1f", size)) + " " + unit
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
	return groupNumber(fmt.Sprintf("%.*f", digits, *value)) + "%"
}

// Bar renders a proportional bar of exactly width characters, or N/A when
// the fraction is unknown.
func Bar(fraction *float64, width int) string {
	if fraction == nil {
		return NA
	}
	clamped := min(100.0, max(0.0, *fraction))
	count := model.RoundHalfEven(clamped / 100.0 * float64(width))
	return strings.Repeat(BarFilled, count) + strings.Repeat(BarEmpty, width-count)
}

func absFloat(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}
