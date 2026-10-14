package config

import (
	"fmt"
	"os"

	"github.com/goccy/go-yaml"
	"github.com/gputop/gputop-go/internal/model"
)

// loadSavedState reads the state: block from the state file, validating it. It
// returns nil when there is no state file, so the caller leaves the config
// untouched. A malformed state file is a warning, not a failure: losing the
// last session's sort order is not a reason to refuse to start.
func loadSavedState(warnings *[]string) *StateConfig {
	path := StatePath()
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var raw map[string]any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		*warnings = append(*warnings, fmt.Sprintf("%s: saved state could not be parsed, ignoring it (%v)", path, err))
		return nil
	}
	section := asSection(raw["state"])
	if section == nil {
		return &StateConfig{}
	}
	var s StateConfig
	applyStateSection(&s, section, warnings)
	validateState(&s, warnings)
	return &s
}

// applyState folds a saved session state over a hand-written configuration.
//
// Only the two values the rest of the program reads through the config are
// moved: the interval, which the sampler is constructed from, and the theme.
// Everything else in state: describes the interface, so the UI reads it from
// the state itself. Precedence is defaults, config file, saved state, command
// line -- and the CLI is applied after this, which is why a one-off
// gputop -i 200 still wins over what the last session settled on.
func applyState(c *Config, state StateConfig, warnings *[]string) {
	c.State = state
	if state.IntervalMs != nil {
		c.IntervalMs = *state.IntervalMs
	}
	if state.Theme != nil {
		c.UI.Theme = *state.Theme
	}
}

// clampInt floors v to [lo, hi].
func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// clampFloat floors v to [lo, hi].
func clampFloat(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// validSortColumn reports whether s names a sortable column.
func validSortColumn(s string) bool { return model.ValidSortColumn(s) }

// validThemeName reports whether name is a known theme.
func validThemeName(name string) bool { return model.ValidTheme(name) }
