package main

import (
	"strings"
	"testing"

	"github.com/gputop/gputop-go/internal/ui"
)

func TestRenderSVGEscapesProcessText(t *testing.T) {
	rows := []ui.Row{{Cells: []ui.Cell{{Text: `a<b&c "quoted"`}}}}
	svg := renderSVG("title <unsafe>", rows, 20, 1)
	if strings.Contains(svg, "a<b&c") || strings.Contains(svg, "<unsafe>") {
		t.Fatalf("SVG contains unescaped text:\n%s", svg)
	}
	for _, want := range []string{"a&lt;b&amp;c &quot;quoted&quot;", "title &lt;unsafe&gt;"} {
		if !strings.Contains(svg, want) {
			t.Fatalf("SVG misses %q:\n%s", want, svg)
		}
	}
}

func TestRenderSVGDrawsReverseVideo(t *testing.T) {
	rows := []ui.Row{{Cells: []ui.Cell{{Text: " 1 ", Color: "#e6e6e6|reverse"}}}}
	svg := renderSVG("reverse", rows, 3, 1)
	for _, want := range []string{`<rect`, `fill="#e6e6e6"`, `fill="#0d1117"`} {
		if !strings.Contains(svg, want) {
			t.Fatalf("SVG misses %q:\n%s", want, svg)
		}
	}
}

func TestGridRowsSplitsEmbeddedNewlines(t *testing.T) {
	rows := []ui.Row{{Cells: []ui.Cell{{Text: "one\ntwo"}}}}
	grid := gridRows(rows, 10)
	if len(grid) != 2 || grid[0][0].Text != "one" || grid[1][0].Text != "two" {
		t.Fatalf("grid = %#v, want two lines", grid)
	}
}
