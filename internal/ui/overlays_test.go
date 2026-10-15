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
	// The note is the last entry; "esc close" still closes the overlay.
	if !strings.Contains(withNote[len(withNote)-2], "radeontop is not installed") {
		t.Fatalf("penultimate line = %q, want the blocks note", withNote[len(withNote)-2])
	}
}

var _ = model.DefaultThemeName // keep model import if helpers change
