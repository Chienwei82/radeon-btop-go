package ui

import "testing"

// The help overlay groups related keys ("tab / shift+tab", "1 … 9"). Those
// groups are the only entries that may describe more than one canonical
// binding; every other binding must appear verbatim, with the same
// description, or the two tables have drifted.
func TestDefaultHelpDocumentsEverySingleKeyBinding(t *testing.T) {
	groups := []struct {
		entry string
		keys  []string
		// named is false for a range entry ("1 … 9") that covers its keys
		// without spelling each one out.
		named bool
	}{
		{"tab / shift+tab", []string{"tab", "shift+tab"}, true},
		{"1 … 9", []string{"1", "2", "3", "4", "5", "6", "7", "8", "9"}, false},
		{"+ / -", []string{"+", "-"}, true},
		{"s / S", []string{"s", "S"}, true},
		{"up / down", []string{"up", "down"}, true},
	}
	grouped := map[string]bool{}
	for _, g := range groups {
		grouped[g.entry] = true
		for _, k := range g.keys {
			grouped[k] = true
		}
	}

	entries := DefaultHelp("signal the selected process")
	byKey := map[string]string{}
	for _, e := range entries {
		if e.Keys != "" {
			byKey[e.Keys] = e.Description
		}
	}
	for _, g := range groups {
		if _, ok := byKey[g.entry]; !ok {
			t.Fatalf("help misses the grouped entry %q", g.entry)
		}
		if !g.named {
			continue
		}
		for _, k := range g.keys {
			if !containsWord(g.entry, k) {
				t.Fatalf("help group %q does not name the binding %q", g.entry, k)
			}
		}
	}
	for _, b := range KeyBindings {
		if grouped[b.Key] {
			continue
		}
		desc, ok := byKey[b.Key]
		if !ok {
			t.Fatalf("binding %q is not documented verbatim in the help", b.Key)
		}
		if desc != b.Description {
			t.Fatalf("binding %q says %q in the help and %q in KeyBindings", b.Key, desc, b.Description)
		}
	}
}

// The footer is curated, not complete, but every key it advertises must be a
// real binding: a hint for a key that does nothing is a broken promise.
func TestFooterHintsUseOnlyDocumentedKeys(t *testing.T) {
	documented := map[string]bool{}
	for _, b := range KeyBindings {
		documented[b.Key] = true
	}
	for _, h := range footerHints {
		switch h.Key {
		case "1-9/tab":
			for _, k := range []string{"1", "9", "tab"} {
				if !documented[k] {
					t.Fatalf("footer hint %q advertises undocumented key %q", h.Key, k)
				}
			}
		case "+-":
			for _, k := range []string{"+", "-"} {
				if !documented[k] {
					t.Fatalf("footer hint %q advertises undocumented key %q", h.Key, k)
				}
			}
		default:
			if !documented[h.Key] {
				t.Fatalf("footer hint %q advertises undocumented key %q", h.Label, h.Key)
			}
		}
	}
}

func containsWord(s, word string) bool {
	for i := 0; i+len(word) <= len(s); i++ {
		if s[i:i+len(word)] == word {
			return true
		}
	}
	return false
}
