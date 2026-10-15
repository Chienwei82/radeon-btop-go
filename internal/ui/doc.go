package ui

// Package ui is the btop-style terminal interface (Phase 2).
//
// The layering contract: every panel builder is a pure function of a
// model snapshot returning a string, testable without a terminal.
// bubbletea (the event loop) and lipgloss (borders, joins, colour)
// only frame those strings; no I/O happens on the TUI goroutine.
