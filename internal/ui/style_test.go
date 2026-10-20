package ui

import "testing"

// One grammar, shared with tools/screenshots: the colour key is a colour plus
// optional "|bold" / "|reverse" modifiers, never a colour that happens to
// contain a pipe.
func TestParseCellStyleSplitsColourAndModifiers(t *testing.T) {
	cases := []struct {
		key     string
		colour  string
		bold    bool
		reverse bool
	}{
		{"#e6e6e6", "#e6e6e6", false, false},
		{"#e6e6e6|bold", "#e6e6e6", true, false},
		{"#e6e6e6|reverse", "#e6e6e6", false, true},
		{"#61afef|reverse|bold", "#61afef", true, true},
		{"|bold", "", true, false},
		{"#fff||", "#fff", false, false},
		{"", "", false, false},
	}
	for _, c := range cases {
		got := ParseCellStyle(c.key)
		if got.Colour != c.colour || got.Bold != c.bold || got.Reverse != c.reverse {
			t.Errorf("ParseCellStyle(%q) = %+v, want {%q %v %v}",
				c.key, got, c.colour, c.bold, c.reverse)
		}
	}
}
