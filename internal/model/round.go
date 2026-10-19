package model

import "math"

// RoundHalfEven rounds to the nearest integer, breaking ties towards the
// even neighbour.
//
// This is Python's round() semantics, which the reference implementation
// uses for every bar fill, ramp index and colour interpolation. Go's
// math.Round rounds half away from zero, so using it (or the common
// int(x+0.5) idiom, which is half-up) renders a bar one cell wider than the
// reference at exact .5 boundaries whose integer part is even:
//
//	 5% of width 10 -> 0.5 -> reference fills 0 cells, half-up fills 1
//	25% of width 10 -> 2.5 -> reference fills 2 cells, half-up fills 3
//
// The function lives in the model layer because both readers (PCIe link
// speed in 0.1 GT/s) and ui (bars, gradients) need the same rule and the
// layering forbids readers -> ui.
func RoundHalfEven(x float64) int {
	if math.IsNaN(x) || math.IsInf(x, 0) {
		// NaN has no nearest integer and infinities overflow int. Both are
		// impossible for a clamped 0-100 percentage, and zero is the only
		// answer that cannot produce a negative repeat count in a caller.
		return 0
	}
	floor := math.Floor(x)
	fraction := x - floor
	switch {
	case fraction > 0.5:
		return int(floor) + 1
	case fraction < 0.5:
		return int(floor)
	case int64(floor)%2 == 0:
		return int(floor)
	default:
		return int(floor) + 1
	}
}
