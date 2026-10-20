package ui

import (
	"strings"
	"testing"

	"github.com/gputop/gputop-go/internal/model"
)

// RED: the overlays do not exist yet.
func TestHelpOverlayListsKeysInColumns(t *testing.T) {
	lines := PlainLines(HelpOverlay(DefaultHelp("kill the selected process"), DefaultTheme))
	if len(lines) < len(DefaultHelp("x")) {
		t.Fatalf("help lines = %d, want at least %d", len(lines), len(DefaultHelp("x")))
	}
	if !strings.Contains(lines[0], "gputop keys") {
		t.Fatalf("title = %q, want %q", lines[0], "gputop keys")
	}
	joined := strings.Join(lines, "\n")
	for _, want := range []string{"quit", "next / previous GPU", "this help", "esc close"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("help misses %q:\n%s", want, joined)
		}
	}
	for _, line := range lines {
		if n := len([]rune(line)); n > HelpWidth {
			t.Fatalf("help line %q is %d cells, want <= %d", line, n, HelpWidth)
		}
	}
}

func TestHelpHidesNothingEveryBindingIsDocumented(t *testing.T) {
	// The reference groups related keys on one line ("1 … 9", "+ / -"), so
	// coverage is on the keys, not on each description verbatim: a binding
	// that silently disappears from the key reference is the failure mode.
	joined := strings.Join(PlainLines(HelpOverlay(DefaultHelp("x"), DefaultTheme)), "\n")
	for _, b := range KeyBindings {
		if len(b.Key) == 1 && b.Key[0] >= '1' && b.Key[0] <= '9' {
			continue // documented by the "1 … 9" entry
		}
		if !strings.Contains(joined, b.Key) {
			t.Fatalf("binding %q (%q) is not in the help", b.Key, b.Description)
		}
	}
}

// A modal is a screen of its own, so it is framed like the dashboard rather
// than floating as loose text over it.
func TestOverlaysAreFramedLikeTheDashboard(t *testing.T) {
	help := PlainLines(HelpOverlay(DefaultHelp("signal the selected process"), DefaultTheme))
	if !strings.HasPrefix(help[0], "╭─ gputop keys ") {
		t.Fatalf("help top border = %q, want a framed title", help[0])
	}
	if !strings.HasPrefix(help[len(help)-1], "╰") {
		t.Fatalf("help bottom border = %q, want a closing rule", help[len(help)-1])
	}

	filter := PlainLines(FilterPrompt("llama", DefaultTheme))
	if !strings.HasPrefix(filter[0], "╭─ Filter processes ") {
		t.Fatalf("filter top border = %q, want a framed title", filter[0])
	}

	kill := PlainLines(KillConfirm(testProcessNamed(1001, "llama-server"), DefaultTheme))
	if !strings.HasPrefix(kill[0], "╭─ Send a signal to pid 1001? ") {
		t.Fatalf("kill top border = %q, want the target in the frame", kill[0])
	}
	for name, lines := range map[string][]string{"help": help, "filter": filter, "kill": kill} {
		for _, line := range lines {
			if !strings.HasPrefix(line, "╭") && !strings.HasPrefix(line, "│") && !strings.HasPrefix(line, "╰") {
				t.Fatalf("%s overlay line is outside its frame: %q", name, line)
			}
		}
	}
}

func TestKillConfirmDescribesTheTarget(t *testing.T) {
	p := testProcessNamed(1001, "llama-server")
	p.User = "david"
	lines := PlainLines(KillConfirm(p, DefaultTheme))
	joined := strings.Join(lines, "\n")
	for _, want := range []string{"1001", "david", "llama-server", "SIGTERM", "SIGKILL", "cancel"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("kill confirm misses %q:\n%s", want, joined)
		}
	}
	for _, line := range lines {
		if n := len([]rune(line)); n > KillWidth {
			t.Fatalf("kill line %q is %d cells, want <= %d", line, n, KillWidth)
		}
	}
}

func TestKillConfirmBodyCarriesUsageNotZero(t *testing.T) {
	p := testProcessNamed(1001, "llama-server")
	body := strings.Join(PlainLines(KillConfirm(p, DefaultTheme)), "\n")
	if !strings.Contains(body, "5.0%") {
		t.Fatalf("kill body misses engine percent:\n%s", body)
	}
	if !strings.Contains(body, "1.0 KiB") {
		t.Fatalf("kill body misses vram:\n%s", body)
	}
}

func TestFilterPromptShowsPlaceholderAndHints(t *testing.T) {
	lines := PlainLines(FilterPrompt("llama", DefaultTheme))
	joined := strings.Join(lines, "\n")
	for _, want := range []string{"Filter processes", "llama", "name, command, user or pid", "enter apply", "esc"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("filter prompt misses %q:\n%s", want, joined)
		}
	}
	for _, line := range lines {
		if n := len([]rune(line)); n > FilterWidth {
			t.Fatalf("filter line %q is %d cells, want <= %d", line, n, FilterWidth)
		}
	}
}

func TestHelpBlocksNoteAppendsWhenPresent(t *testing.T) {
	withNote := PlainLines(HelpOverlay(DefaultHelp("k", "blocks: radeontop is not installed"), DefaultTheme))
	without := PlainLines(HelpOverlay(DefaultHelp("k"), DefaultTheme))
	if len(withNote) != len(without)+1 {
		t.Fatalf("note lines = %d, want %d", len(withNote), len(without)+1)
	}
	// The note sits above the way out, inside the frame.
	joined := strings.Join(withNote, "\n")
	if !strings.Contains(joined, "radeontop is not installed") {
		t.Fatalf("help misses the blocks note:\n%s", joined)
	}
	if !strings.Contains(withNote[len(withNote)-2], "esc") {
		t.Fatalf("penultimate line = %q, want the way out", withNote[len(withNote)-2])
	}
}

var _ = model.DefaultThemeName // keep model import if helpers change
