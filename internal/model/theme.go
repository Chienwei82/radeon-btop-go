package model

// ThemeNames lists the colour themes the interface can apply, in a stable
// order. It is the Go counterpart of the reference ui.theme.THEMES keys.
//
// It lives in the domain model -- beside SortColumns, for the same reason --
// rather than in the presentation layer: config validation must be able to
// reject an unknown theme name, and it must do so without depending on the ui
// package, which does not exist until Phase 2. The palettes themselves are
// presentation and stay in ui; only the closed set of valid names is domain.
var ThemeNames = []string{"default", "dracula", "gruvbox", "monochrome", "radeon"}

// DefaultThemeName is applied when none is configured or the configured one is
// unknown.
const DefaultThemeName = "default"

// ValidTheme reports whether name is a known theme.
func ValidTheme(name string) bool {
	for _, t := range ThemeNames {
		if t == name {
			return true
		}
	}
	return false
}
