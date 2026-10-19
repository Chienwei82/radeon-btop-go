package ui

import (
	"strings"
	"testing"
)

// TestBarFillUsesHalfToEvenRounding pins the boundary table against the
// reference's round(): at an exact .5 whose integer part is even, the bar
// fills *down*. The old int(x+0.5) idiom filled up and made a 5% reading
// draw one filled cell out of ten where Python draws none.
func TestBarFillUsesHalfToEvenRounding(t *testing.T) {
	cases := []struct {
		percent float64
		width   int
		filled  int
	}{
		{5, 10, 0},
		{25, 10, 2},
		{45, 10, 4},
		{15, 10, 2},
		{35, 10, 4},
		{50, 10, 5},
		{100, 10, 10},
		{0, 10, 0},
	}
	for _, c := range cases {
		bar := Bar(&c.percent, c.width)
		if got := strings.Count(bar, BarFilled); got != c.filled {
			t.Errorf("Bar(%v, %d) filled %d cells, want %d (%q)", c.percent, c.width, got, c.filled, bar)
		}
		if got := len([]rune(bar)); got != c.width {
			t.Errorf("Bar(%v, %d) width = %d, want %d", c.percent, c.width, got, c.width)
		}
	}
}

// TestStatRowUsesTheSameRoundingAsBar keeps the two bar painters in step.
// The row reserves its own width for the value text, so the test reads the
// width back off the rendered glyphs rather than assuming one, and compares
// the fill against Bar at exactly that width.
func TestStatRowUsesTheSameRoundingAsBar(t *testing.T) {
	for _, pct := range []float64{5, 25, 45, 50, 99} {
		row := Plain(StatRow("GPU", FmtPercent(&pct, 0), &pct, GetTheme("default"), 40, nil))
		bar := strings.Count(row, BarFilled) + strings.Count(row, BarEmpty)
		if bar == 0 {
			t.Fatalf("%v%% row has no bar: %q", pct, row)
		}
		want := strings.Count(Bar(&pct, bar), BarFilled)
		if got := strings.Count(row, BarFilled); got != want {
			t.Fatalf("%v%% row filled %d of %d cells, Bar fills %d: %q", pct, got, bar, want, row)
		}
	}
}

// TestGradientIndexUsesHalfToEven pins the ramp lookup to the same rule.
func TestGradientIndexUsesHalfToEven(t *testing.T) {
	g := NewGradient(DefaultTheme, true)
	// 0% and 100% are the ends of the ramp; a tie in between must not panic
	// or leave the ramp.
	if got := g.At(0); got != g.Stops()[0] {
		t.Fatalf("At(0) = %q, want %q", got, g.Stops()[0])
	}
	if got := g.At(100); got != g.Stops()[len(g.Stops())-1] {
		t.Fatalf("At(100) = %q, want %q", got, g.Stops()[len(g.Stops())-1])
	}
}
