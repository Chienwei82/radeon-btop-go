package ui

import "testing"

// RED: the key model does not exist yet.
func TestAllThirtyBindingsAreHandled(t *testing.T) {
	view := NewViewState()
	for _, b := range KeyBindings {
		if b.Key == "esc" {
			// esc with no filter text is a no-op (Unknown), not an
			// action: there is nothing to clear. With text it clears.
			view.Filter.Text = "x"
			if action := view.HandleKey(b.Key); action != ActionClearFilter {
				t.Fatalf("esc with filter text must clear, got %v", action)
			}
			view.Filter.Text = ""
			continue
		}
		if action := view.HandleKey(b.Key); action == ActionUnknown {
			t.Fatalf("binding %q (%s) not handled", b.Key, b.Description)
		}
	}
	// The canonical 30: q, tab, shift+tab, o, /, h/?, 1-9, R, +/-, m, p,
	// s/S, r, esc, a, t, c, up/down, k.
	if len(KeyBindings) != 30 {
		t.Fatalf("bindings = %d, want 30", len(KeyBindings))
	}
}

func TestSortCycling(t *testing.T) {
	view := NewViewState()
	first := view.Sort.Column
	view.HandleKey("s")
	if view.Sort.Column == first {
		t.Fatal("s must advance the sort column")
	}
	view.HandleKey("S")
	if view.Sort.Column != first {
		t.Fatalf("S must go back, got %q want %q", view.Sort.Column, first)
	}
}

func TestIntervalClamp(t *testing.T) {
	view := NewViewState()
	view.IntervalS = 600
	view.HandleKey("+")
	if view.IntervalS >= 600 {
		t.Fatal("+ must shorten the interval")
	}
	for i := 0; i < 5000; i++ {
		view.HandleKey("-")
	}
	if view.IntervalS > 600 {
		t.Fatalf("interval = %v, must clamp at 600s", view.IntervalS)
	}
	for i := 0; i < 5000; i++ {
		view.HandleKey("+")
	}
	if view.IntervalS < 0.05 {
		t.Fatalf("interval = %v, must clamp at the floor", view.IntervalS)
	}
}

func TestThemeCycling(t *testing.T) {
	view := NewViewState()
	first := view.ThemeName
	view.HandleKey("m")
	if view.ThemeName == first {
		t.Fatal("m must cycle the theme")
	}
}
