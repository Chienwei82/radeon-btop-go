// Command screenshots renders the README screenshots from the same pure
// screen function the interface uses.
//
// The machine it draws is synthetic and deterministic: a Navi 21 card beside
// a Cezanne APU, five GPU clients, a minute of plausible history and the SCPP
// tables a real card publishes. A screenshot taken from whatever the build
// machine happened to be running would not be reproducible, and one taken
// from a busy desktop is not a fair picture of the interface.
//
// The output is SVG text, not a bitmap: the graphs, bars and colours stay
// sharp at any zoom, and the files diff like text because they are text. Run
// it inside the dev image so the host stays free of a Go toolchain:
//
//	./scripts/podev.sh screenshots
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/gputop/gputop-go/internal/ui"
)

// shotMode selects the view state and optional overlay of one screenshot.
type shotMode int

const (
	modeDetail shotMode = iota
	modeTree
	modeOverview
	modeNarrow
	modeHelp
)

// shot is one SVG the README embeds.
type shot struct {
	name   string
	title  string
	width  int
	height int
	mode   shotMode
}

// shots is the gallery, in README order.
var shots = []shot{
	{name: "main", title: "gputop: one Radeon RX 6800 under load", width: 120, height: 48, mode: modeDetail},
	{name: "process-tree", title: "gputop: process tree with per-engine GPU time", width: 120, height: 48, mode: modeTree},
	{name: "multi-gpu", title: "gputop: overview of a discrete GPU and an APU", width: 120, height: 24, mode: modeOverview},
	{name: "narrow", title: "gputop in a 64-column terminal", width: 64, height: 27, mode: modeNarrow},
	{name: "help", title: "gputop: the key reference", width: 120, height: 48, mode: modeHelp},
}

func main() {
	outDir := flag.String("out", filepath.Join("docs", "screenshots"), "directory to write the SVG files into")
	flag.Parse()
	if err := run(*outDir); err != nil {
		fmt.Fprintf(os.Stderr, "screenshots: %v\n", err)
		os.Exit(1)
	}
}

// run renders every shot into outDir.
func run(outDir string) error {
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return fmt.Errorf("create %s: %w", outDir, err)
	}
	base := machineInput()
	for _, s := range shots {
		rows := s.render(base)
		path := filepath.Join(outDir, s.name+".svg")
		if err := os.WriteFile(path, []byte(renderSVG(s.title, rows, s.width, s.height)), 0o644); err != nil {
			return fmt.Errorf("write %s: %w", path, err)
		}
		fmt.Printf("wrote %s\n", path)
	}
	return nil
}

// render builds the view state for one shot and renders the screen exactly as
// the event loop would, through ui.RenderScreen.
func (s shot) render(base ui.LayoutInput) []ui.Row {
	in := base
	in.Width, in.Height = s.width, s.height
	in.View = *ui.NewViewState()
	switch s.mode {
	case modeTree:
		in.View.Tree = true
	case modeOverview:
		in.View.Overview = true
	case modeNarrow:
		// A 64-column terminal is for the graph and the sensors; the process
		// table is the first thing that yields, exactly as the layout does
		// below its breakpoints.
		in.View.ShowProcesses = false
	case modeHelp:
		// The help box is a screen of its own: centring it on an empty canvas
		// keeps the reference readable instead of layering it over a dashboard.
		return overlayRows(nil, helpBox(), s.width, s.height)
	}
	return ui.RenderScreen(in)
}

// helpBox is the full key reference as the `h` overlay draws it.
func helpBox() []ui.Row {
	return ui.HelpOverlay(ui.DefaultHelp("send SIGTERM or SIGKILL to the selected process"), ui.DefaultTheme)
}
