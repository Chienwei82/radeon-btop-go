package ui

import (
	"fmt"
	"strings"

	"github.com/gputop/gputop-go/internal/model"
)

// LabelWidth reserves the left-hand label column, wide enough for
// "temp junction".
const LabelWidth = 14

// Cell is one styled run inside a Row: plain text plus a colour key into
// the theme. Keeping colour as a key (not an escape code) is what keeps
// panel builders pure and golden-testable: Plain() strips to text, and
// the bubbletea layer resolves keys to lipgloss styles once.
type Cell struct {
	Text  string
	Color string
}

// Row is one rendered panel line: styled runs that join to exactly the
// panel width (or less at the minimum-bar floor).
type Row struct {
	Cells []Cell
}

// Plain strips styling: the text a golden test pins.
func Plain(r Row) string {
	var sb strings.Builder
	for _, c := range r.Cells {
		sb.WriteString(c.Text)
	}
	return sb.String()
}

// PlainLines strips a multi-line panel to text lines.
func PlainLines(rows []Row) []string {
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, Plain(r))
	}
	return out
}

// FitBarWidth chooses a bar width that fits the space a row has. The bar
// is sized from the measured value text rather than a constant, which is
// what stops a long reading from wrapping into the next row.
func FitBarWidth(available, valueReserve int, label int) int {
	if label <= 0 {
		label = LabelWidth
	}
	if w := available - label - valueReserve - 1; w > 6 {
		return w
	}
	return 6
}

// StatRow builds one "label  bar  value" line that fits available columns.
//
// An unavailable reading draws an empty bar and prints N/A. A full bar for
// a missing value would be the single most misleading thing this UI does.
func StatRow(label, value string, fraction *float64, th Theme, available int, gradient *Gradient) Row {
	return StatRowWidth(label, value, fraction, th, available, gradient, LabelWidth)
}

// StatRowWidth is StatRow with a caller-chosen label column, for panels whose
// labels are not the sensor panel's ("temp junction" wide, or the blocks
// panel's longer still).
func StatRowWidth(label, value string, fraction *float64, th Theme, available int, gradient *Gradient, labelWidth int) Row {
	var ramp Gradient
	if gradient != nil {
		ramp = *gradient
	} else {
		ramp = NewGradient(th, true)
	}
	var cells []Cell
	cells = append(cells, Cell{Text: padWidthRight(label, labelWidth), Color: th.Muted})
	width := FitBarWidth(available, DisplayWidth(value)+1, labelWidth)
	if fraction == nil {
		cells = append(cells, Cell{Text: strings.Repeat(BarEmpty, width), Color: th.Track})
		cells = append(cells, Cell{Text: " " + NA, Color: th.Muted})
		return Row{Cells: cells}
	}
	clamped := min(100.0, max(0.0, *fraction))
	filled := model.RoundHalfEven(clamped / 100.0 * float64(width))
	cells = append(cells, Cell{Text: strings.Repeat(BarFilled, filled), Color: ramp.At(clamped)})
	cells = append(cells, Cell{Text: strings.Repeat(BarEmpty, width-filled), Color: th.Track})
	cells = append(cells, Cell{Text: " " + value, Color: th.Text})
	return Row{Cells: cells}
}

// PoolRow builds a memory-pool row: "label  bar  used / total (pct%)".
func PoolRow(label string, pool model.MemoryPool, th Theme, available int, gradient *Gradient) Row {
	pct := pool.Percent()
	var pctF *float64
	if pct != nil {
		v := float64(*pct)
		pctF = &v
	}
	usedS := NA
	if pool.Used != nil {
		usedS = FmtBytes(pool.Used)
	}
	totalS := NA
	if pool.Total != nil {
		totalS = FmtBytes(pool.Total)
	}
	value := usedS + " / " + totalS
	if pct != nil {
		value += fmt.Sprintf(" (%.0f%%)", float64(*pct))
	}
	return StatRow(label, value, pctF, th, available, gradient)
}
