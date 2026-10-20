package ui

import (
	"testing"

	"charm.land/lipgloss/v2"
)

// Resolving a cell's style is the hot path of a repaint: every cell used to
// parse its colour key and build a fresh lipgloss.Style. One style per key is
// enough for a frame, and the palette is a closed set (theme keys plus the
// gradient stops).
func TestStyleCellCachesOneStylePerColourKey(t *testing.T) {
	app := NewApp(AppOptions{Config: appConfig(false)})
	cells := []Cell{
		{Text: "a", Color: DefaultTheme.Text},
		{Text: "b", Color: DefaultTheme.Text},
		{Text: "c", Color: DefaultTheme.Accent + "|bold"},
		{Text: "d", Color: DefaultTheme.Accent + "|bold"},
	}
	for range 3 {
		for _, c := range cells {
			_ = app.styleCell(c)
		}
	}
	if got := len(app.styles); got != 2 {
		t.Fatalf("cached styles = %d, want 2 (one per colour key)", got)
	}
}

// BenchmarkAppStyleCell measures the cache hit: the frame after the first
// resolves each key once.
func BenchmarkAppStyleCell(b *testing.B) {
	app := NewApp(AppOptions{Config: appConfig(false)})
	cell := Cell{Text: "2,200 MHz", Color: DefaultTheme.Text}
	b.ReportAllocs()
	for b.Loop() {
		_ = app.styleCell(cell)
	}
}

// BenchmarkCellStyleConstruction is the work the cache avoids per cell:
// parse the key, build a style, render the run. Comparing it with
// BenchmarkAppStyleCell is the measurement behind the cache.
func BenchmarkCellStyleConstruction(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		parsed := ParseCellStyle("#e6e6e6")
		style := lipgloss.NewStyle().Foreground(lipgloss.Color(parsed.Colour))
		_ = style.Render("2,200 MHz")
	}
}

// BenchmarkRenderRows measures a whole repaint at the reference's pty size,
// which is the cost the productContext promises to keep low.
func BenchmarkRenderRows(b *testing.B) {
	app := NewApp(AppOptions{Config: appConfig(false)})
	in := screenInput()
	in.Width, in.Height = 120, 40
	rows := RenderScreen(in)
	b.ReportAllocs()
	for b.Loop() {
		_ = app.renderRows(rows, 120, 40)
	}
}
