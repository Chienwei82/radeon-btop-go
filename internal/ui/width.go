package ui

import (
	"strings"

	"github.com/clipperhouse/displaywidth"
)

// DisplayWidth is the number of terminal cells s occupies.
//
// A terminal counts cells, not runes: a CJK glyph or an emoji is two columns
// wide and a combining mark is none. Measuring with len([]rune) drew a panel
// that claimed to fit and wrapped instead.
func DisplayWidth(s string) int { return displaywidth.String(s) }

// CutWidth narrows s to at most width cells, marking a cut with an ellipsis
// so a truncated reading never reads as a complete one.
func CutWidth(s string, width int) string {
	if width <= 0 {
		return ""
	}
	return displaywidth.TruncateString(s, width, "…")
}

// truncateWidth cuts without a marker. It is the hard-bound variant used by
// fitRow, where the contract is "never wider than the terminal" rather than
// prose that must announce its missing tail.
func truncateWidth(s string, width int) string {
	if width <= 0 {
		return ""
	}
	return displaywidth.TruncateString(s, width, "")
}

// padWidthLeft right-aligns s in width cells (numbers line up on the digit).
func padWidthLeft(s string, width int) string {
	s = CutWidth(s, width)
	if n := width - DisplayWidth(s); n > 0 {
		return strings.Repeat(" ", n) + s
	}
	return s
}

// padWidthRight left-aligns s in width cells.
func padWidthRight(s string, width int) string {
	s = CutWidth(s, width)
	if n := width - DisplayWidth(s); n > 0 {
		return s + strings.Repeat(" ", n)
	}
	return s
}
