package config

import (
	"fmt"
	"os"

	"github.com/goccy/go-yaml"
)

// SearchPaths returns the config file locations searched when no explicit
// path is given, in precedence order (first found wins).
func SearchPaths() []string {
	var paths []string
	if env := os.Getenv(ConfigEnv); env != "" {
		paths = append(paths, env)
	}
	home, err := os.UserHomeDir()
	if err == nil && home != "" {
		paths = append(paths, home+"/.config/gputop/gputop.yaml")
		paths = append(paths, home+"/.gputoprc")
	}
	return paths
}

// StatePath returns the file the session state is read from and written to.
// Resolution order is the $GPUTOP_STATE environment variable, then the
// default location. It is resolved on every call so that changing the
// environment (as the test suite does) is honoured.
func StatePath() string {
	if env := os.Getenv(StateEnv); env != "" {
		return env
	}
	return expandHome(DefaultStatePath)
}

// expandHome replaces a leading ~ with the user's home directory.
func expandHome(path string) string {
	if len(path) == 0 || path[0] != '~' {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	return home + path[1:]
}

// Load reads and validates a configuration file.
//
// An empty path searches the default locations and folds in the session state
// gputop saved for itself; a named file is taken as the whole configuration
// and the saved state is not layered on top, so a scripted run does not depend
// on whatever the last interactive session left behind.
//
// It never returns an error: a missing, unreadable or malformed file yields
// defaults plus warnings, because a monitor must start even with a broken
// config.
func Load(path string) Config {
	var warnings []string
	withState := path == ""

	raw, chosen := readDocument(path, &warnings)

	for name, value := range raw {
		if !knownRootKey(name) {
			if _, isSection := value.(map[string]any); isSection {
				warnings = append(warnings, name+": unknown section ignored")
			} else {
				warnings = append(warnings, name+": unknown key ignored")
			}
		}
	}

	c := Default()
	c.Warnings = nil
	applyDocument(&c, raw, &warnings)
	validate(&c, &warnings)

	if withState {
		if state := loadSavedState(&warnings); state != nil {
			applyState(&c, *state, &warnings)
		}
	}

	_ = chosen
	c.Warnings = warnings
	return c
}

// knownRootKey reports whether a top-level key is one this build reads.
func knownRootKey(name string) bool {
	switch name {
	case "interval_ms", "history_points", "gpu", "process", "ui", "blocks", "alerts", "log", "state":
		return true
	default:
		return false
	}
}

// readDocument reads the chosen YAML file into a generic map. An empty path
// searches SearchPaths; a missing file simply yields an empty document (and,
// when a path was named explicitly, a "not found" warning). A file that cannot
// be read or parsed is reported and yields an empty document, never a panic.
func readDocument(path string, warnings *[]string) (map[string]any, string) {
	candidates := SearchPaths()
	if path != "" {
		candidates = []string{path}
	}
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		data, err := os.ReadFile(expandHome(candidate))
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			*warnings = append(*warnings, fmt.Sprintf("%s: could not be read (%v)", candidate, err))
			return map[string]any{}, ""
		}
		var raw map[string]any
		if err := yaml.Unmarshal(data, &raw); err != nil {
			*warnings = append(*warnings, fmt.Sprintf("%s: could not be parsed (%v)", candidate, err))
			return map[string]any{}, ""
		}
		if raw == nil {
			raw = map[string]any{}
		}
		return raw, candidate
	}
	if path != "" {
		*warnings = append(*warnings, path+": not found, using defaults")
	}
	return map[string]any{}, ""
}

// asSection coerces a decoded value into a mapping, treating anything that is
// not a mapping as empty so a section written as a scalar or a list is data
// (\"absent\"), not a crash.
func asSection(value any) map[string]any {
	if m, ok := value.(map[string]any); ok {
		return m
	}
	return nil
}
