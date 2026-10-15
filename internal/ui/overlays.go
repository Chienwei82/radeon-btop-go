package ui

import (
	"fmt"

	"github.com/gputop/gputop-go/internal/model"
)

// Overlay box widths. Fixed rather than content-driven: a confirmation
// whose box changes size as the user types is a confirmation that reflows
// under the cursor (the reference's own note).
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

// HelpOverlay renders the key reference: title, one line per entry with
// the keys in a fixed column, and the way out.
func HelpOverlay(entries []HelpEntry, th Theme) []Row {
	out := []Row{{Cells: []Cell{{Text: "gputop keys", Color: th.Accent + "|bold"}}}}
	for _, e := range entries {
		text := padRunesRight(e.Keys, 16) + e.Description
		// The note line has no keys and reads as prose, not as a binding.
		color := ""
		if e.Keys != "" {
			color = th.Muted
		}
		out = append(out, Row{Cells: []Cell{{
			Text:  cutRunes(text, HelpWidth),
			Color: color,
		}}})
	}
	out = append(out, Row{Cells: []Cell{{Text: "esc close", Color: th.Muted}}})
	return out
}

// KillConfirm renders the signal confirmation: who is the target, what it
// is using, and the two signals as explicit keys. They are offered as two
// keys rather than one "kill" because they are not variants of one action:
// one can be caught and one cannot.
func KillConfirm(p model.GpuProcess, th Theme) []Row {
	title := fmt.Sprintf("Send a signal to pid %d?", p.PID)
	body := []string{
		"  user     " + p.User,
		"  command  " + p.Command(),
		fmt.Sprintf("  engine   %s", FmtPercent(pctPtr(p.EnginePercent()), 1)),
		fmt.Sprintf("  memory   %s  (VRAM %s, GTT %s)",
			FmtBytes(bytesPtr2(p.MemoryUsed())), FmtBytes(p.VramUsed), FmtBytes(p.GttUsed)),
	}
	out := []Row{{Cells: []Cell{{Text: title, Color: th.Accent + "|bold"}}}}
	for _, line := range body {
		out = append(out, Row{Cells: []Cell{{Text: cutRunes(line, KillWidth), Color: th.Text}}})
	}
	out = append(out, Row{Cells: []Cell{{Text: cutRunes("t SIGTERM  k SIGKILL  esc cancel", KillWidth), Color: th.Muted}}})
	return out
}

// FilterPrompt renders the filter editor: the title, the current text
// (the prompt opens with the existing filter rather than making the user
// retype it), and the keys that resolve it.
func FilterPrompt(current string, th Theme) []Row {
	field := ">▌"
	if current != "" {
		field = "> " + current + "▌"
	}
	return []Row{
		{Cells: []Cell{{Text: "Filter processes", Color: th.Accent + "|bold"}}},
		{Cells: []Cell{{Text: cutRunes(field, FilterWidth), Color: th.Text}}},
		{Cells: []Cell{{Text: cutRunes("name, command, user or pid", FilterWidth), Color: th.Muted}}},
		{Cells: []Cell{{Text: "enter apply  ·  esc clear", Color: th.Muted}}},
	}
}

func pctPtr(v model.Percent) *model.Percent { return &v }

func bytesPtr2(v model.Bytes) *model.Bytes { return &v }
