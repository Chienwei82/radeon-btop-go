// Package ui is the btop-style terminal interface (Phases 2-3).
//
// The layering contract: every panel builder is a pure function of a model
// snapshot returning []Row, testable without a terminal. A Row is a list of
// Cell{Text, Color} where Color is a theme *key*, not an escape sequence --
// that is what lets Plain() strip a row to the text a golden test pins, and
// it is why bubbletea (the event loop) and lipgloss (borders, joins, colour)
// only have to resolve keys to styles and frame the rows. No I/O happens on
// the TUI goroutine.
//
// The whole screen is one pure function, RenderScreen(LayoutInput) []Row, so
// every responsive rule (which panel is dropped first, when the optional row
// may appear) is a test rather than a terminal experiment.
package ui
