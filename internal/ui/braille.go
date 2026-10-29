package ui

import (
	"strings"
)

// BrailleBase is the first code point of the braille block; dot bits are
// OR-ed into this base.
const BrailleBase = 0x2800

// BrailleBits maps [row][column] to the bit that lights that dot.
//
// The Unicode braille pattern is not a linear grid, which is the usual
// source of off-by-one braille renderers: dots 1-3 and 7 form the left
// column, dots 4-6 and 8 the right one.
var BrailleBits = [4][2]int{
	{0x01, 0x08}, // row 0: dots 1 and 4
	{0x02, 0x10}, // row 1: dots 2 and 5
	{0x04, 0x20}, // row 2: dots 3 and 6
	{0x40, 0x80}, // row 3: dots 7 and 8
}

// ScaleMode selects the graph's vertical axis.
type ScaleMode int

const (
	// ScalePercent fixes the axis to 0-100.
	ScalePercent ScaleMode = iota
	// ScaleAuto fits the data.
	ScaleAuto
)

// BrailleText rasterises a series into plain braille text.
//
// Split out from any widget so the rasteriser can be tested at explicit
// dimensions without mounting anything.
//
//   - samples: values oldest-first. nil is a gap and is left empty rather
//     than interpolated across, so a reading the hardware did not provide
//     shows as a hole instead of a straight line between two points.
//   - width: cells across. height: cells down; each cell is 2x4 dots.
//   - scale: "percent" fixes the axis to 0-100; "auto" fits the data.
//
// Returns styled-agnostic text exactly width columns by height rows: the
// caller applies the gradient colour per cell. Filled cells accumulate per
// dot column rather than per sample: a 300-sample history in a 58-cell
// panel puts roughly five samples in each dot column, and every one of
// them paints the same run of dots from its own value down to the bottom,
// so the drawing only depends on the highest point and the peak value of
// each column.
func BrailleText(samples []*float64, width, height int, gradient Gradient, track string, scale ScaleMode) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	dotsHigh := height * 4
	dotCols := width * 2

	var lo, hi float64
	first := true
	if len(samples) > 0 {
		for _, s := range samples {
			if s == nil {
				continue
			}
			v := *s
			if first {
				lo, hi = v, v
				first = false
			} else {
				lo, hi = min(lo, v), max(hi, v)
			}
		}
	}
	if first {
		lo, hi = 0, 100
	}
	if scale == ScaleAuto {
		if !first {
			if hi-lo < 1e-9 {
				mid := (hi + lo) / 2
				lo, hi = mid-50, mid+50
			} else {
				padding := (hi - lo) * 0.1
				lo, hi = lo-padding, hi+padding
			}
		}
	} else {
		lo, hi = 0, 100
	}
	span := hi - lo
	if span < 1e-9 {
		span = 1
	}

	toRow := func(value float64) int {
		position := (value - lo) / span
		position = min(1.0, max(0.0, position))
		return dotsHigh - 1 - int(position*float64(dotsHigh-1)+0.5)
	}

	// Reduce each dot column to its peak: for every sample that lands in
	// the column, paint dots from the sample's row down to the bottom.
	peaks := make([]float64, dotCols)
	has := make([]bool, dotCols)
	if len(samples) > 0 {
		for i, s := range samples {
			if s == nil {
				continue
			}
			column := 0
			if len(samples) > 1 {
				column = int(float64(i) / float64(len(samples)-1) * float64(dotCols-1))
			} else {
				column = dotCols - 1 // a lone sample sits at the right edge
			}
			if !has[column] || *s > peaks[column] {
				peaks[column], has[column] = *s, true
			}
		}
	}

	_ = gradient
	_ = track
	var sb strings.Builder
	for row := range height {
		for col := range width {
			bits := 0
			for dotRow := range 4 {
				globalRow := row*4 + dotRow
				for dotCol := range 2 {
					globalCol := col*2 + dotCol
					if !has[globalCol] {
						continue
					}
					if globalRow >= toRow(peaks[globalCol]) {
						bits |= BrailleBits[dotRow][dotCol]
					}
				}
			}
			sb.WriteRune(rune(BrailleBase + bits))
		}
		if row < height-1 {
			sb.WriteByte('\n')
		}
	}
	return sb.String()
}

// BrailleCells is the per-cell dot pattern plus the peak value behind it,
// so the caller can colour each cell from the gradient.
type BrailleCells struct {
	// Bits holds the dot bits per cell, row-major.
	Bits [][]int
	// Peak holds the peak value per cell for gradient colouring.
	Peak [][]float64
	// HasPeak reports whether a cell has any sample behind it.
	HasPeak [][]bool
}
