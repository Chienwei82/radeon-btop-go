package ui

import (
	"strings"
	"testing"
)

// The terminal counts cells, not runes: a CJK glyph or an emoji occupies two
// columns. Measuring with len([]rune) drew a "full" panel that wrapped.
func TestDisplayWidthCountsTerminalCells(t *testing.T) {
	cases := []struct {
		text string
		want int
	}{
		{"", 0},
		{"abc", 3},
		{"日本語", 6},
		{"e\u0301", 1}, // e + combining acute is one grapheme
		{"🐍", 2},
		{"a日🐍", 5},
	}
	for _, c := range cases {
		if got := DisplayWidth(c.text); got != c.want {
			t.Errorf("DisplayWidth(%q) = %d, want %d", c.text, got, c.want)
		}
	}
}

// CutWidth is the truncation choke point: the result fits the cell budget and
// says it was cut.
func TestCutWidthMarksACutWithinTheBudget(t *testing.T) {
	got := CutWidth("日本語", 4)
	if w := DisplayWidth(got); w > 4 {
		t.Fatalf("CutWidth = %q (%d cells), want at most 4", got, w)
	}
	if !strings.HasSuffix(got, "…") {
		t.Fatalf("CutWidth = %q, want an ellipsis marking the cut", got)
	}
	if whole := CutWidth("abc", 3); whole != "abc" {
		t.Fatalf("CutWidth left a fitting string changed: %q", whole)
	}
}

// fitRow is the last-resort width contract; it must cut by cells so a wide
// grapheme cannot leak past the terminal edge.
func TestFitRowMeasuresDisplayCells(t *testing.T) {
	row := Row{Cells: []Cell{{Text: "日本語"}, {Text: "🐍🐍"}}}
	fitted := fitRow(row, 7)
	if w := DisplayWidth(Plain(fitted)); w > 7 {
		t.Fatalf("fitRow produced %d cells, want at most 7: %q", w, Plain(fitted))
	}
}

// A wide title must not push the right corner off the frame.
func TestPanelBoxKeepsItsWidthWithWideText(t *testing.T) {
	rows := PanelBox("日本語GPU", []Row{{Cells: []Cell{{Text: "🐍🐍🐍🐍"}}}}, 24, DefaultTheme)
	for i, row := range rows {
		if w := DisplayWidth(Plain(row)); w != 24 {
			t.Fatalf("line %d is %d cells, want 24: %q", i, w, Plain(row))
		}
	}
}
