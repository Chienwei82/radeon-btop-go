package ui

import (
	"strings"
	"testing"
)

// The screen has to read as separate panels, not one wall of text. Every
// primary panel is framed and its title sits in the top border.
func TestScreenDrawsEveryPrimaryPanelAsAFrame(t *testing.T) {
	in := screenInput()
	in.Width, in.Height = 120, 40
	joined := strings.Join(plainRows(RenderScreen(in)), "\n")
	for _, title := range []string{"GPU busy", "Sensors", "VRAM", "GTT", "Processes"} {
		if !strings.Contains(joined, "╭─ "+title) {
			t.Fatalf("screen misses the framed %q panel:\n%s", title, joined)
		}
	}
}

// Two side-by-side frames leave four vertical rules on every shared body row.
// That is the visible division the old layout lacked.
func TestMainRowHasAVisibleColumnDivision(t *testing.T) {
	in := screenInput()
	in.Width, in.Height = 120, 40
	for _, line := range plainRows(RenderScreen(in)) {
		if strings.Count(line, "│") >= 4 && strings.Contains(line, "Core") {
			return
		}
	}
	t.Fatalf("no row carries both panel frames:\n%s", strings.Join(plainRows(RenderScreen(in)), "\n"))
}

// VRAM and GTT are two panels on one row, the way the reference lays them
// out. Stacking them wasted half the width and twice the height.
func TestMemoryGraphsShareOneRow(t *testing.T) {
	in := screenInput()
	in.Width, in.Height = 120, 40
	for _, line := range plainRows(RenderScreen(in)) {
		if strings.Contains(line, "╭─ VRAM") && strings.Contains(line, "╭─ GTT") {
			return
		}
	}
	t.Fatalf("VRAM and GTT are not on one row:\n%s", strings.Join(plainRows(RenderScreen(in)), "\n"))
}

// A key is a target, not prose: it gets the accent and bold, the label stays
// muted, and the two are separate runs so a terminal can show the difference.
func TestKeyHintsPaintKeysApartFromTheirLabels(t *testing.T) {
	row := keyHints(DefaultTheme, 100)
	if !strings.HasPrefix(Plain(row), "q quit") {
		t.Fatalf("hint line = %q, want it to start with q quit", Plain(row))
	}
	key, label := false, false
	for _, c := range row.Cells {
		if c.Text == "q" && c.Color == DefaultTheme.Accent+"|bold" {
			key = true
		}
		if c.Text == " quit" && c.Color == DefaultTheme.Muted {
			label = true
		}
	}
	if !key || !label {
		t.Fatalf("keys and labels are not styled apart: %#v", row.Cells)
	}
}

// The key reference is the one screen where mixing keys and prose is most
// confusing; it must make the same distinction as the hint line.
func TestHelpOverlayPaintsKeysApartFromTheirDescriptions(t *testing.T) {
	rows := HelpOverlay(DefaultHelp("signal the selected process"), DefaultTheme)
	key, description := false, false
	for _, row := range rows {
		for _, c := range row.Cells {
			if strings.TrimRight(c.Text, " ") == "q" && c.Color == DefaultTheme.Accent+"|bold" {
				key = true
			}
			if c.Text == "quit" && c.Color == DefaultTheme.Text {
				description = true
			}
		}
	}
	if !key || !description {
		t.Fatalf("help keys and descriptions are not styled apart: %#v", PlainLines(rows))
	}
}

// Two frames on one row must share their baseline. A short body in one of
// them would leave the other's bottom border a row lower and the row would
// stop reading as a pair of panels.
func TestOptionalRowFramesShareTheirBaseline(t *testing.T) {
	in := profileInput()
	b := blockSample()
	in.Snapshot.Devices[0].Blocks = &b
	in.Width, in.Height = 160, 50
	lines := plainRows(RenderScreen(in))
	top := -1
	for i, line := range lines {
		if strings.Contains(line, "╭─ Blocks") && strings.Contains(line, "╭─ Power profile") {
			top = i
			break
		}
	}
	if top < 0 {
		t.Fatalf("optional panels are not framed on one row:\n%s", strings.Join(lines, "\n"))
	}
	for _, line := range lines[top+1:] {
		if strings.Contains(line, "╰") {
			if n := strings.Count(line, "╰"); n != 2 {
				t.Fatalf("frames close on different rows (%d bottom corners):\n%s", n, strings.Join(lines, "\n"))
			}
			return
		}
	}
	t.Fatalf("optional frames never close:\n%s", strings.Join(lines, "\n"))
}
