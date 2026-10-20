package ui

import (
	"strings"
	"testing"
)

// A panel must be a frame, not just a heading: the title lives in the top
// border and both sides are drawn, which is what makes adjacent panels read as
// separate regions instead of one column of loose text.
func TestPanelBoxFramesTheBodyWithARoundedTitleBorder(t *testing.T) {
	body := []Row{{Cells: []Cell{{Text: "body", Color: DefaultTheme.Text}}}}
	rows := PanelBox("GPU busy", body, 24, DefaultTheme)
	if len(rows) != 3 {
		t.Fatalf("rows = %d, want 3 (top, body, bottom)", len(rows))
	}
	lines := PlainLines(rows)
	for i, line := range lines {
		if n := len([]rune(line)); n != 24 {
			t.Fatalf("line %d is %d cells, want 24: %q", i, n, line)
		}
	}
	if !strings.HasPrefix(lines[0], "╭─ GPU busy ") || !strings.HasSuffix(lines[0], "╮") {
		t.Fatalf("top border = %q, want a title embedded in it", lines[0])
	}
	if !strings.HasPrefix(lines[1], "│ ") || !strings.HasSuffix(lines[1], " │") {
		t.Fatalf("body = %q, want a bordered row", lines[1])
	}
	if !strings.Contains(lines[1], "body") {
		t.Fatalf("body = %q, want the content", lines[1])
	}
	if !strings.HasPrefix(lines[2], "╰") || !strings.HasSuffix(lines[2], "╯") {
		t.Fatalf("bottom border = %q, want a rounded corner", lines[2])
	}
}

// A long title is cut before it can push the right corner off the line: the
// width contract belongs to the frame, not to the text it happens to carry.
func TestPanelBoxKeepsItsWidthWhenTheTitleIsTooLong(t *testing.T) {
	for _, width := range []int{4, 5, 8, 12, 20} {
		body := []Row{{Cells: []Cell{{Text: strings.Repeat("x", 40)}}}}
		rows := PanelBox("a very long panel title", body, width, DefaultTheme)
		for i, line := range PlainLines(rows) {
			if n := len([]rune(line)); n != width {
				t.Fatalf("width %d line %d is %d cells: %q", width, i, n, line)
			}
		}
	}
}

// Borders carry the accent; a body run keeps the colour the panel chose for
// it. A frame that repainted its content would defeat the theme keys.
func TestPanelBoxPaintsBordersAndPreservesBodyColours(t *testing.T) {
	rows := PanelBox("T", []Row{{Cells: []Cell{{Text: "x", Color: "#123456"}}}}, 10, DefaultTheme)
	if got := rows[0].Cells[0].Color; got != DefaultTheme.Accent {
		t.Fatalf("top-left border colour = %q, want the accent %q", got, DefaultTheme.Accent)
	}
	if got := rows[2].Cells[0].Color; got != DefaultTheme.Accent {
		t.Fatalf("bottom-left border colour = %q, want the accent %q", got, DefaultTheme.Accent)
	}
	found := false
	for _, c := range rows[1].Cells {
		if c.Text == "x" && c.Color == "#123456" {
			found = true
		}
	}
	if !found {
		t.Fatalf("body colour lost: %#v", rows[1].Cells)
	}
}

// Title-less frames are still valid (a separator panel in a test, a waiting
// panel before the first sample): they draw a plain rounded rule.
func TestPanelBoxWithoutATitleIsStillAFrame(t *testing.T) {
	rows := PanelBox("", nil, 8, DefaultTheme)
	lines := PlainLines(rows)
	if len(lines) != 2 {
		t.Fatalf("rows = %d, want top and bottom", len(lines))
	}
	if lines[0] != "╭──────╮" || lines[1] != "╰──────╯" {
		t.Fatalf("plain frame = %q / %q", lines[0], lines[1])
	}
}
