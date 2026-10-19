package model

import "testing"

// TestRoundHalfEvenMatchesPythonRound pins the boundary table that made the
// difference visible: the reference's round() is half-to-even, so an exact
// .5 lands on the even neighbour, not always upwards.
func TestRoundHalfEvenMatchesPythonRound(t *testing.T) {
	cases := []struct {
		in   float64
		want int
	}{
		{0.0, 0},
		{0.5, 0},
		{1.5, 2},
		{2.5, 2},
		{3.5, 4},
		{4.5, 4},
		{5.5, 6},
		// The bar-fill table from the reference's own widths.
		{0.5, 0}, // 5% of 10
		{2.5, 2}, // 25% of 10
		{4.5, 4}, // 45% of 10
		{1.5, 2}, // 15% of 10
		{3.5, 4}, // 35% of 10
		// Not ties: ordinary nearest-integer rounding.
		{0.49, 0},
		{0.51, 1},
		{2.49, 2},
		{2.51, 3},
		// Negative ties follow the same rule (-0.5 -> 0, -1.5 -> -2), which
		// matters for a lerp that overshoots below zero.
		{-0.5, 0},
		{-1.5, -2},
		{-2.5, -2},
	}
	for _, c := range cases {
		if got := RoundHalfEven(c.in); got != c.want {
			t.Errorf("RoundHalfEven(%v) = %d, want %d", c.in, got, c.want)
		}
	}
}

// TestRoundHalfEvenNeverProducesANegativeRepeat guards the callers that turn
// the result into a strings.Repeat count.
func TestRoundHalfEvenNeverProducesANegativeRepeat(t *testing.T) {
	for _, value := range []float64{-0.5, -0.4, 0, 0.4, 0.5} {
		if got := RoundHalfEven(value); got < 0 {
			t.Errorf("RoundHalfEven(%v) = %d, want >= 0", value, got)
		}
	}
}
