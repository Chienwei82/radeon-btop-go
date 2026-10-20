package ui

import "strings"

// CellStyle is a theme colour key resolved into what a renderer needs.
//
// The grammar is shared by every consumer of Row: the TUI resolves it to a
// lipgloss style and tools/screenshots resolves it to an SVG fill. Keeping
// one parser is what stops the screenshot from disagreeing with the terminal
// about what "|reverse" means.
type CellStyle struct {
	Colour  string
	Bold    bool
	Reverse bool
}

// ParseCellStyle splits a cell colour key into its colour and modifiers.
//
// The suffixes are attributes of the style, not part of the colour: handing
// "#e6e6e6|reverse" to lipgloss as a colour silently drops the reverse video,
// which is how the active GPU badge lost its highlight. Empty runs ("||")
// are ignored so a key may carry padding without becoming a colour.
func ParseCellStyle(key string) CellStyle {
	var style CellStyle
	for _, part := range strings.Split(key, "|") {
		switch part {
		case "":
			// "|bold|" and similar have an empty run; ignore it.
		case "bold":
			style.Bold = true
		case "reverse":
			style.Reverse = true
		default:
			style.Colour = part
		}
	}
	return style
}
