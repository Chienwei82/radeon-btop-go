package ui

import "strings"

// Box-drawing glyphs for PanelBox. Rounded corners rather than square ones:
// they read as regions at a glance, which is the whole point of drawing a
// frame in a terminal that has no background fills to lean on.
const (
	boxTopLeft     = "╭"
	boxTopRight    = "╮"
	boxBottomLeft  = "╰"
	boxBottomRight = "╯"
	boxHorizontal  = "─"
	boxVertical    = "│"
)

// boxOverheadX is what a PanelBox spends horizontally on its frame: two
// border cells and one padding cell on each side. Callers size a panel's
// content from this, never from the outer width.
const boxOverheadX = 4

// boxOverheadY is the frame's vertical cost: one top border and one bottom.
const boxOverheadY = 2

// PanelBox wraps body in a rounded, accent-coloured frame whose title is
// embedded in the top border.
//
// The frame is drawn here, in the pure row model, rather than by lipgloss or a
// layout engine: it is part of the screen function, so the divisions between
// panels are golden-testable and the event loop still only resolves colours.
// Every returned row is exactly width cells, which is what lets two frames sit
// side by side with their borders aligned.
//
// Below four columns there is no room for a frame and a body, so the content
// is returned trimmed rather than drawn wider than the terminal.
func PanelBox(title string, body []Row, width int, th Theme) []Row {
	if width <= 0 {
		return nil
	}
	if width < boxOverheadX {
		out := make([]Row, 0, len(body))
		for _, r := range body {
			out = append(out, fitRow(r, width))
		}
		return out
	}

	contentWidth := width - boxOverheadX
	out := make([]Row, 0, len(body)+boxOverheadY)
	out = append(out, boxTop(title, width, th))
	for _, r := range body {
		if contentWidth > 0 {
			r = fitRow(r, contentWidth)
		} else {
			// At exactly four columns the frame is two borders and two
			// padding cells; there is no content column left.
			r = Row{}
		}
		out = append(out, boxBodyRow(r, contentWidth, th))
	}
	out = append(out, boxBottom(width, th))
	return out
}

// boxTop builds "╭─ title ─────╮", truncating the title rather than letting it
// push the right corner off the line.
func boxTop(title string, width int, th Theme) Row {
	frame := th.frameColor()
	title = strings.TrimSpace(title)
	maxTitle := width - 6 // corners (2), "─ " (2), and a space either side (2)
	if maxTitle < 1 {
		title = ""
	} else {
		title = CutWidth(title, maxTitle)
	}
	if title == "" {
		return Row{Cells: []Cell{
			{Text: boxTopLeft, Color: frame},
			{Text: strings.Repeat(boxHorizontal, width-2), Color: frame},
			{Text: boxTopRight, Color: frame},
		}}
	}
	fill := width - 5 - DisplayWidth(title)
	return Row{Cells: []Cell{
		{Text: boxTopLeft + boxHorizontal + " ", Color: frame},
		{Text: title, Color: th.Accent + "|bold"},
		{Text: " ", Color: frame},
		{Text: strings.Repeat(boxHorizontal, fill), Color: frame},
		{Text: boxTopRight, Color: frame},
	}}
}

// boxBodyRow draws one bordered line: the frame's verticals and padding are
// themed, the content keeps its own colour keys.
func boxBodyRow(content Row, contentWidth int, th Theme) Row {
	frame := th.frameColor()
	content = padRowTo(content, contentWidth)
	cells := make([]Cell, 0, len(content.Cells)+4)
	cells = append(cells, Cell{Text: boxVertical, Color: frame}, Cell{Text: " "})
	cells = append(cells, content.Cells...)
	cells = append(cells, Cell{Text: " "}, Cell{Text: boxVertical, Color: frame})
	return Row{Cells: cells}
}

// boxBottom is the frame's closing rule.
func boxBottom(width int, th Theme) Row {
	frame := th.frameColor()
	return Row{Cells: []Cell{
		{Text: boxBottomLeft, Color: frame},
		{Text: strings.Repeat(boxHorizontal, width-2), Color: frame},
		{Text: boxBottomRight, Color: frame},
	}}
}
