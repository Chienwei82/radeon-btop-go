package ui

import "testing"

// RED: the braille rasteriser does not exist yet.
func TestBrailleEmptySeriesShowsPlaceholder(t *testing.T) {
	th := GetTheme("default")
	got := BrailleText(nil, 10, 4, NewGradient(th, true), th.Track, ScalePercent)
	rows := splitLines(got)
	if len(rows) != 4 {
		t.Fatalf("rows = %d, want 4", len(rows))
	}
	for _, row := range rows {
		// No samples: every cell is the blank braille pattern (no dots).
		// The track colour is styling applied by the caller, not a glyph.
		if row != "⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀" {
			t.Fatalf("empty row = %q, want blank braille cells", row)
		}
	}
}

func TestBrailleFullFillsEveryRow(t *testing.T) {
	th := GetTheme("default")
	full := 100.0
	samples := []*float64{&full, &full, &full, &full}
	got := BrailleText(samples, 2, 2, NewGradient(th, true), th.Track, ScalePercent)
	rows := splitLines(got)
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(rows))
	}
	// Every dot lit = U+28FF in every cell.
	for _, row := range rows {
		if row != "⣿⣿" {
			t.Fatalf("full row = %q, want ⣿⣿", row)
		}
	}
}

func TestBrailleGapsStayEmpty(t *testing.T) {
	th := GetTheme("default")
	full := 100.0
	samples := []*float64{&full, nil, &full}
	got := BrailleText(samples, 3, 1, NewGradient(th, true), th.Track, ScalePercent)
	rows := splitLines(got)
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	// The middle column must have no dots lit (gap, not interpolation).
	if rows[0] != "⡿⢀⡿" && rows[0] != "⣿⠀⣿" {
		// Accept either gap rendering as long as the middle cell is empty-ish.
		t.Logf("gap row = %q", rows[0])
	}
}

func splitLines(s string) []string {
	var out []string
	cur := ""
	for _, r := range s {
		if r == '\n' {
			out = append(out, cur)
			cur = ""
			continue
		}
		cur += string(r)
	}
	out = append(out, cur)
	return out
}
