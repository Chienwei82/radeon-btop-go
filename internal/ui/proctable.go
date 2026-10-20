package ui

import (
	"strings"

	"github.com/gputop/gputop-go/internal/model"
)

// TableConfig is what the table renderer needs: how wide it may be, which
// client is selected (by identity, so a rebuild cannot move it), which ones
// are the busiest, and the palette their marks are painted with.
type TableConfig struct {
	Width       int
	FullCommand bool
	Selected    *procID
	Top         []model.GpuProcess
	Theme       Theme
}

// minCommandWidth is the floor the command column shrinks to before any
// other column is dropped. Below it a name stops being readable at all.
const minCommandWidth = 6

// RenderTable renders the heading line plus one line per row, every line at
// most cfg.Width cells wide.
//
// The layout degrades instead of overflowing: columns fall off from the
// right (least scannable first — GTT, VRAM, engines), the command column
// absorbs the remaining squeeze, and any pathological leftover is cut from
// the line's end so the width contract holds unconditionally.
func RenderTable(rows []ProcessRow, sort SortState, cfg TableConfig) []Row {
	th := cfg.Theme
	if th.Name == "" {
		th = DefaultTheme
	}
	cols, widths := tableLayout(cfg.Width, sort)
	out := make([]Row, 0, len(rows)+1)
	out = append(out, fitRow(headingRow(th, sort, cols, widths), cfg.Width))
	for _, r := range rows {
		out = append(out, fitRow(processRow(th, r, cols, widths, cfg), cfg.Width))
	}
	return out
}

// headingRow is the column titles line; the active column carries the sort
// marker so the order is readable without a second line of text. Headings
// are left-aligned prose, whatever the data cells below them do.
func headingRow(th Theme, sort SortState, cols []model.SortColumn, widths []int) Row {
	cells := make([]Cell, 0, len(cols)*2)
	for i, c := range cols {
		if i > 0 {
			cells = append(cells, Cell{Text: " "})
		}
		cells = append(cells, Cell{
			Text:  padWidthRight(sort.Heading(c), widths[i]),
			Color: th.Muted,
		})
	}
	return Row{Cells: trimLast(cells)}
}

// processRow renders one client: tree guides fold into the command cell
// (they indent the name, not the table), the marker slots in front of the
// PID, and a marked row is painted with the accent so it is findable at a
// glance — the glyph alone keeps it findable when colour is off.
func processRow(th Theme, pr ProcessRow, cols []model.SortColumn, widths []int, cfg TableConfig) Row {
	marked := cfg.isMarked(pr.Process)
	color := ""
	if marked {
		color = th.Accent + "|bold"
	}
	values := RowCells(pr.Process, cfg.FullCommand)
	cells := make([]Cell, 0, len(cols)*2)
	for i, c := range cols {
		if i > 0 {
			cells = append(cells, Cell{Text: " ", Color: color})
		}
		w := widths[i]
		var text string
		switch {
		case c == model.SortCommand:
			text = padWidthRight(pr.Prefix()+values[columnIndex(c)], w)
		case c == model.SortPID:
			if marked {
				text = TopMarker + padWidthLeft(values[columnIndex(c)], w-1)
			} else {
				text = padWidthLeft(values[columnIndex(c)], w)
			}
		case textColumn(c):
			text = padWidthRight(values[columnIndex(c)], w)
		default:
			text = padWidthLeft(values[columnIndex(c)], w)
		}
		cells = append(cells, Cell{Text: text, Color: color})
	}
	return Row{Cells: trimLast(cells)}
}

// isMarked reports whether the client carries the top/selection marker.
func (c TableConfig) isMarked(p model.GpuProcess) bool {
	if c.Selected != nil && idOf(p) == *c.Selected {
		return true
	}
	for _, t := range c.Top {
		if idOf(t) == idOf(p) {
			return true
		}
	}
	return false
}

// tableLayout picks which columns fit in width and how wide each is.
//
// Order of concessions: drop columns right of the command (GTT first — a
// memory figure loses to a process name every time), then squeeze the
// command to its floor, then give every remaining cell the slack. The PID,
// USER and COMMAND columns and the active sort column never drop: sorting
// by a column that is not on screen is a lie, and identity is the point of
// the table.
func tableLayout(width int, sort SortState) ([]model.SortColumn, []int) {
	cols := append([]model.SortColumn(nil), ColumnOrder...)
	widths := make([]int, len(cols))
	for i, c := range cols {
		widths[i] = ColumnWidths[c]
		if c == model.SortPID {
			widths[i]++ // marker slot
		}
		// The active heading carries " ▼", which can exceed the column.
		if h := DisplayWidth(sort.Heading(c)); h > widths[i] {
			widths[i] = h
		}
	}
	if width <= 0 {
		return cols, widths
	}
	cmd := columnIndex(model.SortCommand)
	for totalWidth(cols, widths) > width {
		dropped := -1
		for i := len(cols) - 1; i > cmd; i-- {
			if cols[i] != sort.Column {
				dropped = i
				break
			}
		}
		if dropped < 0 {
			break
		}
		cols = append(cols[:dropped], cols[dropped+1:]...)
		widths = append(widths[:dropped], widths[dropped+1:]...)
	}
	if over := totalWidth(cols, widths) - width; over > 0 {
		// Only the command can still give, and not below its floor.
		if widths[cmd]-minCommandWidth > over {
			widths[cmd] -= over
		} else {
			widths[cmd] = minCommandWidth
		}
	}
	if slack := width - totalWidth(cols, widths); slack > 0 {
		widths[cmd] += slack
	}
	return cols, widths
}

func totalWidth(cols []model.SortColumn, widths []int) int {
	total := len(cols) - 1 // one separator space between columns
	for _, w := range widths {
		total += w
	}
	return total
}

// fitRow is the last-resort width contract: cut any excess from the end of
// the line's cells, right to left, so a marker or a PID is lost only when
// the terminal is narrower than the identity columns themselves.
func fitRow(r Row, width int) Row {
	if width <= 0 {
		return r
	}
	total := 0
	for _, c := range r.Cells {
		total += DisplayWidth(c.Text)
	}
	for i := len(r.Cells) - 1; i >= 0 && total > width; i-- {
		cellWidth := DisplayWidth(r.Cells[i].Text)
		allowed := cellWidth - (total - width)
		text := truncateWidth(r.Cells[i].Text, allowed)
		total -= cellWidth - DisplayWidth(text)
		r.Cells[i].Text = text
	}
	return r
}

// trimLast drops the last cell's trailing padding: the line ends at its
// content, so goldens carry no trailing whitespace.
func trimLast(cells []Cell) []Cell {
	if n := len(cells); n > 0 {
		cells[n-1].Text = strings.TrimRight(cells[n-1].Text, " ")
	}
	return cells
}

// textColumn reports whether a column is left-aligned prose.
func textColumn(c model.SortColumn) bool {
	return c == model.SortUser || c == model.SortCommand
}

// columnIndex is the column's position in ColumnOrder.
func columnIndex(c model.SortColumn) int {
	for i, oc := range ColumnOrder {
		if oc == c {
			return i
		}
	}
	return -1
}
