package ui

import (
	"testing"
)

func TestAreaEmptySeriesShowsBlankCells(t *testing.T) {
	th := GetTheme("default")
	grad := NewGradient(th, true)
	cells := AreaText(nil, 10, 4, grad, ScalePercent)
	if len(cells) != 4 {
		t.Fatalf("rows = %d, want 4", len(cells))
	}
	for _, row := range cells {
		if len(row) != 10 {
			t.Fatalf("cols = %d, want 10", len(row))
		}
		for _, c := range row {
			if c.HasValue {
				t.Fatalf("empty series cell has value: %#v", c)
			}
			if c.Block != ' ' {
				t.Fatalf("empty series block = %q, want space", c.Block)
			}
		}
	}
}

func TestAreaFullFillsEveryRow(t *testing.T) {
	th := GetTheme("default")
	grad := NewGradient(th, true)
	full := 100.0
	samples := []*float64{&full, &full, &full, &full}
	cells := AreaText(samples, 2, 2, grad, ScalePercent)
	if len(cells) != 2 {
		t.Fatalf("rows = %d, want 2", len(cells))
	}
	for _, row := range cells {
		for _, c := range row {
			if !c.HasValue {
				t.Fatalf("full series cell has no value: %#v", c)
			}
			if c.Block != '█' {
				t.Fatalf("full series block = %q, want █", c.Block)
			}
		}
	}
}

func TestAreaGapsStayEmpty(t *testing.T) {
	th := GetTheme("default")
	grad := NewGradient(th, true)
	full := 100.0
	samples := []*float64{&full, nil, &full}
	cells := AreaText(samples, 3, 1, grad, ScalePercent)
	if len(cells) != 1 {
		t.Fatalf("rows = %d, want 1", len(cells))
	}
	// The middle column must have no value (gap, not interpolation).
	if cells[0][1].HasValue {
		t.Fatalf("gap column has a value: %#v", cells[0][1])
	}
	if cells[0][1].Block != ' ' {
		t.Fatalf("gap column block = %q, want space", cells[0][1].Block)
	}
	// The left and right columns must have values.
	if !cells[0][0].HasValue || !cells[0][2].HasValue {
		t.Fatalf("non-gap columns missing values: %#v", cells[0])
	}
}

func TestAreaPartialBlockAt50Percent(t *testing.T) {
	th := GetTheme("default")
	grad := NewGradient(th, true)
	half := 50.0
	samples := []*float64{&half}
	cells := AreaText(samples, 1, 1, grad, ScalePercent)
	if len(cells) != 1 || len(cells[0]) != 1 {
		t.Fatalf("dimensions = %dx%d, want 1x1", len(cells), len(cells[0]))
	}
	c := cells[0][0]
	if !c.HasValue {
		t.Fatalf("50%% cell has no value: %#v", c)
	}
	// At 50% with height=1 (8 sub-rows), the block should be around ▄ (4/8).
	// Accept a range of partial blocks around the middle.
	if c.Block == ' ' || c.Block == '█' {
		t.Fatalf("50%% block = %q, want a partial block", c.Block)
	}
}

func TestAreaGradientColorsEachCell(t *testing.T) {
	th := GetTheme("default")
	grad := NewGradient(th, true)
	full := 100.0
	low := 10.0
	samples := []*float64{&low, &full}
	cells := AreaText(samples, 2, 1, grad, ScalePercent)
	// The cell with the higher value should have a different colour
	// than the one with the lower value.
	if cells[0][0].Color == cells[0][1].Color {
		t.Fatalf("gradient did not differentiate cell colours: %q == %q",
			cells[0][0].Color, cells[0][1].Color)
	}
}

func TestAreaScalePercentClampsTo0_100(t *testing.T) {
	th := GetTheme("default")
	grad := NewGradient(th, true)
	over := 150.0
	samples := []*float64{&over}
	cells := AreaText(samples, 1, 2, grad, ScalePercent)
	// 150% clamped to 100% → all rows full.
	for _, row := range cells {
		if row[0].Block != '█' {
			t.Fatalf("over-100%% block = %q, want █", row[0].Block)
		}
	}
}

func TestAreaScaleAutoFitsData(t *testing.T) {
	th := GetTheme("default")
	grad := NewGradient(th, true)
	v1 := 42.0
	v2 := 58.0
	samples := []*float64{&v1, &v2}
	cells := AreaText(samples, 2, 4, grad, ScaleAuto)
	// Both columns should have values somewhere in the grid.
	col0Has, col1Has := false, false
	for _, row := range cells {
		if row[0].HasValue {
			col0Has = true
		}
		if row[1].HasValue {
			col1Has = true
		}
	}
	if !col0Has || !col1Has {
		t.Fatalf("auto-scale cells missing values: col0=%v col1=%v", col0Has, col1Has)
	}
}

func TestAreaWidthHeightMatch(t *testing.T) {
	th := GetTheme("default")
	grad := NewGradient(th, true)
	v := 75.0
	samples := []*float64{&v, &v, &v}
	for _, dims := range []struct{ w, h int }{{1, 1}, {5, 3}, {10, 8}, {40, 12}} {
		cells := AreaText(samples, dims.w, dims.h, grad, ScalePercent)
		if len(cells) != dims.h {
			t.Fatalf("%dx%d: rows = %d, want %d", dims.w, dims.h, len(cells), dims.h)
		}
		for i, row := range cells {
			if len(row) != dims.w {
				t.Fatalf("%dx%d row %d: cols = %d, want %d", dims.w, dims.h, i, len(row), dims.w)
			}
		}
	}
}

func TestAreaNoColorModeStillHasBlocks(t *testing.T) {
	// A zero gradient (no-color) should still produce visible blocks.
	var grad Gradient
	full := 100.0
	samples := []*float64{&full}
	cells := AreaText(samples, 1, 1, grad, ScalePercent)
	if cells[0][0].Block != '█' {
		t.Fatalf("no-color full block = %q, want █", cells[0][0].Block)
	}
}

func TestAreaZeroValueShowsLowestBlock(t *testing.T) {
	// A real 0% reading must be visible as the lowest block, not confused
	// with an absent (nil) sample. Honest numbers: present-but-zero is data.
	th := GetTheme("default")
	grad := NewGradient(th, true)
	zero := 0.0
	samples := []*float64{&zero}
	cells := AreaText(samples, 1, 1, grad, ScalePercent)
	c := cells[0][0]
	if !c.HasValue {
		t.Fatalf("0%% cell has no value: %#v", c)
	}
	if c.Block != '▁' {
		t.Fatalf("0%% block = %q, want ▁", c.Block)
	}
}

func TestAreaZeroValueNotConfusedWithGap(t *testing.T) {
	// A present 0% and an absent (nil) sample must render differently.
	th := GetTheme("default")
	grad := NewGradient(th, true)
	zero := 0.0
	samples := []*float64{&zero, nil}
	cells := AreaText(samples, 2, 1, grad, ScalePercent)
	if !cells[0][0].HasValue || cells[0][0].Block != '▁' {
		t.Fatalf("present 0%% = %#v, want visible ▁", cells[0][0])
	}
	if cells[0][1].HasValue || cells[0][1].Block != ' ' {
		t.Fatalf("nil gap = %#v, want empty space", cells[0][1])
	}
}

// ── Benchmarks ───────────────────────────────────────────────────────────

// BenchmarkAreaText measures the rasteriser at a typical graph size (the GPU
// busy chart is ~50 columns wide and a handful of rows tall).
func BenchmarkAreaText(b *testing.B) {
	th := GetTheme("default")
	grad := NewGradient(th, false)
	samples := make([]*float64, 300)
	for i := range samples {
		v := float64((i * 37) % 100)
		samples[i] = &v
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = AreaText(samples, 50, 8, grad, ScalePercent)
	}
}
