package main

import (
	"fmt"
	"math"
	"strings"
	"unicode/utf8"

	"github.com/gputop/gputop-go/internal/ui"
)

// SVG geometry. The cell width is deliberately a float: Fira Code at 14 px is
// 8.4 px per cell, and rounding each x position to a whole pixel would make a
// long line drift by a cell.
const (
	svgFontSize    = 14.0
	svgCharWidth   = 8.4
	svgLineHeight  = 17.0
	svgPadding     = 18.0
	svgBackground  = "#0d1117"
	svgDefaultText = "#e6e6e6"
	svgFontFamily  = "'Fira Code','JetBrains Mono','DejaVu Sans Mono',monospace"
)

// renderSVG turns the rows a panel builder returns into a self-contained SVG.
//
// Every run gets its own <text> at an absolute x, so a font that lacks a
// braille or box-drawing glyph falls back without shifting the rest of the
// line. Colours come straight from the Row cells: no terminal, no lipgloss
// profile and no ANSI parsing are involved.
func renderSVG(title string, rows []ui.Row, cols, lines int) string {
	grid := gridRows(rows, lines)
	width := math.Ceil(2*svgPadding + float64(cols)*svgCharWidth)
	height := math.Ceil(2*svgPadding + float64(lines)*svgLineHeight)

	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" width="%.0f" height="%.0f" viewBox="0 0 %.0f %.0f" role="img">`+"\n",
		width, height, width, height)
	fmt.Fprintf(&b, "  <title>%s</title>\n", xmlEscape(title))
	fmt.Fprintf(&b, "  <rect width=\"100%%\" height=\"100%%\" fill=\"%s\"/>\n", svgBackground)
	fmt.Fprintf(&b, "  <g font-family=\"%s\" font-size=\"%.0f\" xml:space=\"preserve\">\n", svgFontFamily, svgFontSize)
	for line, cells := range grid {
		col := 0
		for _, cell := range cells {
			if cell.Text == "" {
				continue
			}
			fill, bold, reverse := cellStyle(cell.Color)
			x := svgPadding + float64(col)*svgCharWidth
			runWidth := float64(utf8.RuneCountInString(cell.Text)) * svgCharWidth
			y := svgPadding + float64(line)*svgLineHeight
			if reverse {
				fmt.Fprintf(&b, "    <rect x=\"%.2f\" y=\"%.2f\" width=\"%.2f\" height=\"%.2f\" fill=\"%s\"/>\n",
					x, y+1, runWidth, svgLineHeight-2, fill)
				fill = svgBackground
			}
			weight := ""
			if bold {
				weight = ` font-weight="bold"`
			}
			fmt.Fprintf(&b, "    <text x=\"%.2f\" y=\"%.2f\" fill=\"%s\"%s>%s</text>\n",
				x, y+svgFontSize, fill, weight, xmlEscape(cell.Text))
			col += utf8.RuneCountInString(cell.Text)
		}
	}
	b.WriteString("  </g>\n</svg>\n")
	return b.String()
}

// gridRows flattens rows that still carry embedded newlines and caps the
// result at the terminal height. RenderScreen already splits the header, but
// an overlay is not guaranteed to.
func gridRows(rows []ui.Row, height int) [][]ui.Cell {
	out := make([][]ui.Cell, 0, height)
	for _, row := range rows {
		current := make([]ui.Cell, 0, len(row.Cells))
		for _, cell := range row.Cells {
			parts := strings.Split(cell.Text, "\n")
			for i, part := range parts {
				if i > 0 {
					out = append(out, current)
					current = make([]ui.Cell, 0, len(row.Cells))
				}
				if part != "" {
					current = append(current, ui.Cell{Text: part, Color: cell.Color})
				}
			}
		}
		out = append(out, current)
	}
	if len(out) > height {
		out = out[:height]
	}
	return out
}

// cellStyle resolves a theme cell key ("#rrggbb", optionally with "|bold"
// or "|reverse") to an SVG fill. An empty key is the terminal's default
// colour. A reverse-video run is drawn as a coloured rect with background
// text, which is how a terminal renders it.
func cellStyle(color string) (fill string, bold, reverse bool) {
	if color == "" {
		return svgDefaultText, false, false
	}
	for _, part := range strings.Split(color, "|") {
		switch part {
		case "bold":
			bold = true
		case "reverse":
			reverse = true
		default:
			if strings.HasPrefix(part, "#") {
				fill = part
			}
		}
	}
	if fill == "" {
		fill = svgDefaultText
	}
	return fill, bold, reverse
}

// overlayRows centres an overlay box over base the way App.withOverlay does:
// the box replaces the lines it covers rather than drawing a background.
func overlayRows(base, box []ui.Row, width, height int) []ui.Row {
	boxWidth := 0
	for _, row := range box {
		boxWidth = max(boxWidth, utf8.RuneCountInString(ui.Plain(row)))
	}
	boxWidth = min(boxWidth, width)

	out := append([]ui.Row(nil), base...)
	for len(out) < height {
		out = append(out, ui.Row{})
	}
	startY := max(0, (height-len(box))/2)
	left := max(0, (width-boxWidth)/2)
	for i, row := range box {
		y := startY + i
		if y >= height {
			break
		}
		cells := []ui.Cell{{Text: strings.Repeat(" ", left)}}
		cells = append(cells, fitCells(row.Cells, boxWidth)...)
		out[y] = ui.Row{Cells: cells}
	}
	if len(out) > height {
		out = out[:height]
	}
	return out
}

// fitCells truncates a row to width cells, preserving each run's colour.
func fitCells(cells []ui.Cell, width int) []ui.Cell {
	out := make([]ui.Cell, 0, len(cells))
	remaining := width
	for _, cell := range cells {
		if remaining <= 0 {
			break
		}
		runes := []rune(cell.Text)
		if len(runes) > remaining {
			runes = runes[:remaining]
		}
		out = append(out, ui.Cell{Text: string(runes), Color: cell.Color})
		remaining -= len(runes)
	}
	return out
}

// xmlEscape makes panel text safe for an XML text node. Process names are
// user-controlled, so this is not optional.
func xmlEscape(s string) string {
	return xmlEscaper.Replace(s)
}

var xmlEscaper = strings.NewReplacer(
	"&", "&amp;",
	"<", "&lt;",
	">", "&gt;",
	`"`, "&quot;",
)
