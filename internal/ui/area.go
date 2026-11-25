package ui

import "github.com/gputop/gputop-go/internal/model"

// ScaleMode selects the graph's vertical axis.
type ScaleMode int

const (
	// ScalePercent fixes the axis to 0-100. This is what graphRows uses.
	ScalePercent ScaleMode = iota
	// ScaleAuto fits the axis to the data range.
	//
	// Reserved: exercised only by tests. graphRows always passes ScalePercent,
	// so no production path selects this today. Kept as a tested capability so
	// an auto-fitting mode can be wired without rediscovering the edge cases.
	ScaleAuto
)

// AreaCell is one cell in an area chart: the block glyph that represents
// the fill level, the colour from the gradient, and whether a sample sits
// behind this column at all.
type AreaCell struct {
	// Block is the Unicode block character for the fill level.
	Block rune
	// Color is the gradient colour for this cell's value.
	Color string
	// HasValue reports whether a sample exists behind this column.
	HasValue bool
}

// blockRunes maps a sub-row level (0–7) to a partial block glyph.
// Index 0 is the lowest (▁) and 7 is the highest (█).
var blockRunes = [8]rune{'▁', '▂', '▃', '▄', '▅', '▆', '▇', '█'}

// AreaText rasterises a series into a 2-D grid of AreaCells.
//
// Each column maps to one or more samples (oldest-first). The peak value
// in each column determines the fill height. The chart is drawn bottom-up:
// rows below the peak are fully filled (█), and the row at the peak uses
// a partial block for sub-row granularity. Columns with no data (nil
// samples) remain empty (space, HasValue=false) — gaps are never
// interpolated.
//
// The gradient colours each column according to its peak value; the colour is
// constant down a column, so the chart reads as a per-column heat band rather
// than a vertical ramp. When the gradient is zero-valued (no-colour mode),
// cells still carry visible block glyphs.
//
//   - samples: values oldest-first; nil is a gap.
//   - width:   columns across.
//   - height:  rows down.
//   - gradient: colour ramp; zero value means no colour.
//   - scale:   ScalePercent fixes 0–100; ScaleAuto fits the data.
func AreaText(samples []*float64, width, height int, gradient Gradient, scale ScaleMode) [][]AreaCell {
	if width <= 0 || height <= 0 {
		return nil
	}

	// Determine the value range.
	var lo, hi float64
	first := true
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
	if first {
		lo, hi = 0, 100
	}
	if scale == ScaleAuto && !first {
		if hi-lo < 1e-9 {
			mid := (hi + lo) / 2
			lo, hi = mid-50, mid+50
		} else {
			padding := (hi - lo) * 0.1
			lo, hi = lo-padding, hi+padding
		}
	} else {
		lo, hi = 0, 100
	}
	span := hi - lo
	if span < 1e-9 {
		span = 1
	}

	// Reduce samples to one peak per column.
	peaks := make([]float64, width)
	has := make([]bool, width)
	if len(samples) > 0 {
		for i, s := range samples {
			if s == nil {
				continue
			}
			column := 0
			if len(samples) > 1 {
				column = int(float64(i) / float64(len(samples)-1) * float64(width-1))
			} else {
				column = width - 1
			}
			if !has[column] || *s > peaks[column] {
				peaks[column], has[column] = *s, true
			}
		}
	}

	// Build the grid bottom-up.
	grid := make([][]AreaCell, height)
	for row := range grid {
		grid[row] = make([]AreaCell, width)
	}
	subRows := height * 8 // total sub-rows for partial-block granularity

	for col := range width {
		// Resolve this column once: its peak, whether it has data, and the
		// gradient colour (constant down the column). Hoisting the colour out
		// of the row loop avoids recomputing gradient.At for every filled cell.
		var colColor string
		var filled int
		if has[col] {
			norm := (peaks[col] - lo) / span
			norm = min(1.0, max(0.0, norm))
			// How many sub-rows from the bottom are filled. A present sample
			// always fills at least one sub-row: a real 0% must render as the
			// lowest block, never as a gap indistinguishable from an absent
			// (nil) reading.
			filled = max(1, model.RoundHalfEven(norm*float64(subRows)))
			if gradient.Len() > 0 {
				colColor = gradient.At(norm * 100)
			}
		}
		for row := range height {
			cell := AreaCell{Block: ' '}
			if has[col] {
				// This row covers sub-rows [row*8, row*8+8) from the top.
				// Convert to bottom-up: sub-row from bottom = subRows - 1 - (row*8 + subRow).
				rowTop := row * 8
				rowBottom := rowTop + 7
				// Number of filled sub-rows in this row's range.
				// filled counts from the bottom.
				bottomThreshold := subRows - 1 - rowBottom // sub-row index from bottom at row bottom
				if filled > bottomThreshold {
					// At least partially filled.
					partial := filled - bottomThreshold
					if partial >= 8 {
						cell.Block = '█'
					} else if partial > 0 {
						cell.Block = blockRunes[partial-1]
					}
					cell.HasValue = true
					cell.Color = colColor
				}
			}
			grid[row][col] = cell
		}
	}
	return grid
}
