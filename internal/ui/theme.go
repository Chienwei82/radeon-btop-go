// Package ui is the btop-style terminal interface (Phase 2).
//
// The layering contract: every panel builder is a pure function of a
// model snapshot returning a string, testable without a terminal.
// bubbletea (the event loop) and lipgloss (borders, joins, colour)
// only frame those strings; no I/O happens on the TUI goroutine.
package ui

// Theme is a named palette. The graph gradient is computed, not stored:
// interpolated between low/mid/high so the colour tracks the value the
// way a thermometer does instead of jumping between flat colours.
type Theme struct {
	Name   string
	Low    string
	Mid    string
	High   string
	Accent string
	Text   string
	Muted  string
	Track  string
	OK     string
	Warn   string
	Alert  string
}

var (
	// DefaultTheme is the stock palette.
	DefaultTheme = Theme{
		Name: "default", Low: "#4ec9b0", Mid: "#dcdcaa", High: "#f7768e",
		Accent: "#61afef", Text: "#e6e6e6", Muted: "#7f848e", Track: "#3a3f4b",
		OK: "#4ec9b0", Warn: "#e5c07b", Alert: "#f7768e",
	}
	// DraculaTheme is the Dracula palette.
	DraculaTheme = Theme{
		Name: "dracula", Low: "#50fa7b", Mid: "#f1fa8c", High: "#ff5555",
		Accent: "#bd93f9", Text: "#f8f8f2", Muted: "#6272a4", Track: "#44475a",
		OK: "#50fa7b", Warn: "#f1fa8c", Alert: "#ff5555",
	}
	// GruvboxTheme is the Gruvbox palette.
	GruvboxTheme = Theme{
		Name: "gruvbox", Low: "#b8bb26", Mid: "#fabd2f", High: "#fb4934",
		Accent: "#83a598", Text: "#ebdbb2", Muted: "#928374", Track: "#504945",
		OK: "#b8bb26", Warn: "#fabd2f", Alert: "#fb4934",
	}
	// MonochromeTheme is the greyscale palette for --no-color.
	MonochromeTheme = Theme{
		Name: "monochrome", Low: "#2a2a2a", Mid: "#aaaaaa", High: "#ffffff",
		Accent: "#888888", Text: "#ffffff", Muted: "#aaaaaa", Track: "#181818",
		OK: "#2a2a2a", Warn: "#aaaaaa", Alert: "#ffffff",
	}
	// RadeonTheme nods to the AMD brand red at saturation.
	RadeonTheme = Theme{
		Name: "radeon", Low: "#0d0d0d", Mid: "#8a8a8a", High: "#d1495b",
		Accent: "#e04000", Text: "#e6e6e6", Muted: "#9a9a9a", Track: "#0d0d0d",
		OK: "#8a8a8a", Warn: "#e04000", Alert: "#8a1a1a",
	}
)

// Themes lists the available themes in a stable order.
var Themes = map[string]Theme{
	"default":    DefaultTheme,
	"dracula":    DraculaTheme,
	"gruvbox":    GruvboxTheme,
	"monochrome": MonochromeTheme,
	"radeon":     RadeonTheme,
}

// DefaultThemeName is applied when none is configured or the configured
// one is unknown.
const DefaultThemeName = "default"

// GetTheme looks up a theme by name, falling back to the default.
//
// An unknown name must not fail: it comes from a config file or a command
// line the user can typo, and refusing to start over a colour name would
// be absurd.
func GetTheme(name string) Theme {
	if th, ok := Themes[lower(trimSpace(name))]; ok {
		return th
	}
	return DefaultTheme
}

// ThresholdStyle picks a status colour from a percentage.
//
// Used where a gradient would be misleading: a discrete state, not a
// magnitude.
func ThresholdStyle(th Theme, percent *float64) string {
	if percent == nil {
		return th.Muted
	}
	if *percent >= 90.0 {
		return th.Alert
	}
	if *percent >= 70.0 {
		return th.Warn
	}
	return th.OK
}
