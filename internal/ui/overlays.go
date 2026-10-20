package ui

import (
	"fmt"

	"github.com/gputop/gputop-go/internal/model"
)

// Overlay box widths. Fixed rather than content-driven: a confirmation
// whose box changes size as the user types is a confirmation that reflows
// under the cursor (the reference's own note). The values are framed outer
// widths; the body gets width-boxOverheadX columns.
const (
	FilterWidth = 64
	KillWidth   = 76
	HelpWidth   = 74
)

// HelpEntry is one key-reference line: the keys, then what they do.
type HelpEntry struct {
	Keys        string
	Description string
}

// DefaultHelp is the full key reference, mirroring the reference's
// grouping ("1 … 9", "+ / -"). signalHelp is a parameter because what `k`
// does depends on the configuration; blocksNote explains a panel that is
// not showing, so the reason the key reference omits it is visible rather
// than inferred.
func DefaultHelp(signalHelp string, blocksNote ...string) []HelpEntry {
	entries := []HelpEntry{
		{"q", "quit"},
		{"tab / shift+tab", "next / previous GPU"},
		{"1 … 9", "jump to GPU n"},
		{"o", "overview: every GPU at once"},
		{"m", "cycle colour theme"},
		{"+ / -", "sampling interval"},
		{"p", "show or hide the process table"},
		{"s / S", "next / previous sort column"},
		{"r", "reverse the sort"},
		{"R", "sample now"},
		{"/", "filter processes"},
		{"esc", "clear the filter"},
		{"a", "show or hide idle processes"},
		{"t", "process tree on / off"},
		{"c", "full command line"},
		{"up / down", "move the selection"},
		{"k", signalHelp},
		{"h", "this help"},
	}
	if len(blocksNote) > 0 && blocksNote[0] != "" {
		entries = append(entries, HelpEntry{"", blocksNote[0]})
	}
	return entries
}

// HelpKeyColumn is the fixed width of the key column in the key reference.
const HelpKeyColumn = 16

// HelpOverlay renders the key reference: a framed screen with one line per
// entry, the keys in a fixed column, and the way out.
//
// The key column is painted with the accent and bold while the description
// stays in body text, so the eye can scan bindings without reading the prose.
// The modal is framed like the dashboard: it replaces the screen it covers,
// so it should not read as loose text floating over one.
func HelpOverlay(entries []HelpEntry, th Theme) []Row {
	contentWidth := HelpWidth - boxOverheadX
	descWidth := contentWidth - HelpKeyColumn
	body := make([]Row, 0, len(entries)+1)
	for _, e := range entries {
		// The note line has no keys and reads as prose, not as a binding.
		if e.Keys == "" {
			body = append(body, Row{Cells: []Cell{{Text: CutWidth(e.Description, contentWidth), Color: th.Muted}}})
			continue
		}
		keys := padWidthRight(e.Keys, HelpKeyColumn)
		body = append(body, Row{Cells: []Cell{
			{Text: keys, Color: th.Accent + "|bold"},
			{Text: CutWidth(e.Description, descWidth), Color: th.Text},
		}})
	}
	body = append(body, Row{Cells: []Cell{
		{Text: "esc", Color: th.Accent + "|bold"},
		{Text: " close", Color: th.Muted},
	}})
	return PanelBox("gputop keys", body, HelpWidth, th)
}

// KillConfirm renders the signal confirmation: who is the target, what it
// is using, and the two signals as explicit keys. They are offered as two
// keys rather than one "kill" because they are not variants of one action:
// one can be caught and one cannot.
func KillConfirm(p model.GpuProcess, th Theme) []Row {
	title := fmt.Sprintf("Send a signal to pid %d?", p.PID)
	contentWidth := KillWidth - boxOverheadX
	lines := []string{
		"  user     " + p.User,
		"  command  " + p.Command(),
		fmt.Sprintf("  engine   %s", FmtPercent(pctPtr(p.EnginePercent()), 1)),
		fmt.Sprintf("  memory   %s  (VRAM %s, GTT %s)",
			FmtBytes(bytesPtr2(p.MemoryUsed())), FmtBytes(p.VramUsed), FmtBytes(p.GttUsed)),
	}
	body := make([]Row, 0, len(lines)+1)
	for _, line := range lines {
		body = append(body, Row{Cells: []Cell{{Text: CutWidth(line, contentWidth), Color: th.Text}}})
	}
	body = append(body, Row{Cells: []Cell{
		{Text: "t", Color: th.Accent + "|bold"}, {Text: " SIGTERM  ", Color: th.Muted},
		{Text: "k", Color: th.Accent + "|bold"}, {Text: " SIGKILL  ", Color: th.Muted},
		{Text: "esc", Color: th.Accent + "|bold"}, {Text: " cancel", Color: th.Muted},
	}})
	return PanelBox(title, body, KillWidth, th)
}

// FilterPrompt renders the filter editor: the title, the current text
// (the prompt opens with the existing filter rather than making the user
// retype it), and the keys that resolve it.
func FilterPrompt(current string, th Theme) []Row {
	contentWidth := FilterWidth - boxOverheadX
	field := ">▌"
	if current != "" {
		field = "> " + current + "▌"
	}
	body := []Row{
		{Cells: []Cell{{Text: CutWidth(field, contentWidth), Color: th.Text}}},
		{Cells: []Cell{{Text: CutWidth("name, command, user or pid", contentWidth), Color: th.Muted}}},
		{Cells: []Cell{
			{Text: "enter", Color: th.Accent + "|bold"},
			{Text: " apply  ·  ", Color: th.Muted},
			{Text: "esc", Color: th.Accent + "|bold"},
			{Text: " clear", Color: th.Muted},
		}},
	}
	return PanelBox("Filter processes", body, FilterWidth, th)
}

func pctPtr(v model.Percent) *model.Percent { return &v }

func bytesPtr2(v model.Bytes) *model.Bytes { return &v }
