package config

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/goccy/go-yaml"
	"github.com/goccy/go-yaml/ast"
	"github.com/goccy/go-yaml/parser"
)

// StateSection is the top-level key this module owns. Nothing else in the
// file is ever written.
const StateSection = "state"

// stateSection is the on-disk shape of the state block. Field order is the
// order the file reads in -- what it looks like, then what it shows -- and is
// explicit rather than derived from StateConfig so the two can evolve
// independently.
//
// Pointer fields are omitted when nil: nil means "not recorded", and writing
// a substitute (an empty string, a zero) would turn "the user never chose a
// theme" into "the user chose no theme" on the next read.
type stateSection struct {
	Theme           *string `yaml:"theme,omitempty"`
	IntervalMs      *int    `yaml:"interval_ms,omitempty"`
	ProcessSort     *string `yaml:"process_sort,omitempty"`
	ProcessSortRev  *bool   `yaml:"process_sort_reverse,omitempty"`
	ProcessFilter   string  `yaml:"process_filter"`
	ShowIdle        bool    `yaml:"show_idle_processes"`
	ShowProcessTree bool    `yaml:"show_process_tree"`
	FullCommand     bool    `yaml:"full_command"`
	Overview        bool    `yaml:"overview"`
	GpuIndex        int     `yaml:"gpu_index"`
}

// stateTable projects a StateConfig onto the file's shape.
func stateTable(state StateConfig) stateSection {
	return stateSection{
		Theme:           state.Theme,
		IntervalMs:      state.IntervalMs,
		ProcessSort:     state.Sort,
		ProcessSortRev:  state.SortReverse,
		ProcessFilter:   state.Filter,
		ShowIdle:        state.ShowIdle,
		ShowProcessTree: state.ProcessTree,
		FullCommand:     state.FullCommand,
		Overview:        state.Overview,
		GpuIndex:        state.GpuIndex,
	}
}

// SaveState writes state into the file's state: block.
//
// It returns "" on success, or a message saying why the state could not be
// saved. A monitor that has been running for hours is not the place to raise:
// the data is still on screen, and the caller turns the message into a footer
// warning.
//
// Two properties matter more than the format. The rest of the document is
// preserved -- writing state must not clobber a hand-edited gpu.names table
// or drop the user's comments -- so the parsed AST is edited in place. And a
// document that cannot be parsed is not written at all: merging into an empty
// document would replace the user's configuration with a bare state: block.
func SaveState(state StateConfig) string {
	target := StatePath()
	original, err := os.ReadFile(target)
	exists := err == nil
	if err != nil && !os.IsNotExist(err) {
		return fmt.Sprintf("%s: state could not be read (%v)", target, err)
	}

	file, parseErr := parser.ParseBytes(original, parser.ParseComments)
	if parseErr != nil {
		return fmt.Sprintf(
			"%s: state not saved because the file is not valid YAML (%v); "+
				"gputop read it as empty and will save again once it parses", target, parseErr)
	}

	payload := ""
	if documentHasState(original) && len(file.Docs) > 0 {
		node, nodeErr := yaml.ValueToNode(stateTable(state))
		if nodeErr != nil {
			return fmt.Sprintf("%s: state could not be encoded (%v)", target, nodeErr)
		}
		statePath, pathErr := yaml.PathString("$." + StateSection)
		// ReplaceWithNode reports no error for a path that is not there on
		// some documents and panics on others, so presence is checked first
		// rather than inferred from the error.
		if pathErr == nil {
			if replaced := replaceNode(statePath, file, node); replaced {
				payload = file.String()
			}
		}
	}
	if payload == "" {
		// There is no state: block to replace, so the section is added at the
		// end of the document the user wrote. Everything above it, comments
		// included, is the original bytes.
		payload = appendStateSection(string(original), state, exists)
	}
	if _, verifyErr := parser.ParseBytes([]byte(payload), parser.ParseComments); verifyErr != nil {
		return fmt.Sprintf("%s: state not saved because the result would not parse (%v)", target, verifyErr)
	}
	return atomicWrite(target, payload)
}

// appendStateSection adds a state: block to a document that has none.
func appendStateSection(document string, state StateConfig, exists bool) string {
	if !exists {
		document = ""
	}
	body, err := yaml.Marshal(stateTable(state))
	if err != nil {
		// Marshalling a struct of scalars cannot fail in practice; if it ever
		// does, the caller's verification step refuses to write.
		return document
	}
	var b strings.Builder
	b.WriteString(document)
	if document != "" {
		if !strings.HasSuffix(document, "\n") {
			b.WriteByte('\n')
		}
		b.WriteByte('\n')
	}
	b.WriteString(StateSection + ":\n")
	for _, line := range indentNormalised(string(body)) {
		b.WriteString("  " + line + "\n")
	}
	return b.String()
}

// indentNormalised strips the common leading indentation the marshaller
// produced, so the section is written at exactly the two spaces a nested
// mapping needs. Without this, a marshalled struct that arrives indented is
// nested further than it should be: still valid YAML, but not the file a
// person would have written.
func indentNormalised(body string) []string {
	lines := strings.Split(strings.TrimRight(body, "\n"), "\n")
	common := -1
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		lead := len(line) - len(strings.TrimLeft(line, " "))
		if common < 0 || lead < common {
			common = lead
		}
	}
	if common <= 0 {
		return lines
	}
	for i, line := range lines {
		if len(line) >= common {
			lines[i] = line[common:]
		}
	}
	return lines
}

// documentHasState reports whether the document already has a state key.
// Decoding is how the key set is inspected everywhere else in this package,
// and it is the only reliable test: the AST path API does not report a
// missing path consistently.
func documentHasState(document []byte) bool {
	var raw map[string]any
	if err := yaml.Unmarshal(document, &raw); err != nil {
		return false
	}
	_, ok := raw[StateSection]
	return ok
}

// replaceNode swaps the state subtree for node. The library panics on some
// documents whose path resolves to nothing, so a panic is treated as "could
// not replace" and the caller falls back to appending the section.
func replaceNode(path *yaml.Path, file *ast.File, node ast.Node) (replaced bool) {
	defer func() {
		if recover() != nil {
			replaced = false
		}
	}()
	return path.ReplaceWithNode(file, node) == nil
}

// atomicWrite replaces target with payload without ever leaving it
// half-written: the content goes to a temporary file in the same directory,
// is flushed, keeps the permissions the target already had, and is renamed
// over it, which POSIX makes atomic. The directory is flushed too, because
// the rename is only durable once the directory entry is.
func atomicWrite(target, payload string) string {
	directory := filepath.Dir(target)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return fmt.Sprintf("%s: state could not be saved (%v)", target, err)
	}
	mode := os.FileMode(0o600)
	if info, err := os.Stat(target); err == nil {
		// A config may have been tightened deliberately; a rename must not
		// loosen it. A file that does not exist yet is created private.
		mode = info.Mode().Perm()
	}
	temporary, err := os.CreateTemp(directory, "."+filepath.Base(target)+".*.tmp")
	if err != nil {
		return fmt.Sprintf("%s: state could not be saved (%v)", target, err)
	}
	name := temporary.Name()
	discard := func() {
		_ = temporary.Close()
		_ = os.Remove(name)
	}
	if _, err := io.WriteString(temporary, payload); err != nil {
		discard()
		return fmt.Sprintf("%s: state could not be saved (%v)", target, err)
	}
	if err := temporary.Sync(); err != nil {
		discard()
		return fmt.Sprintf("%s: state could not be saved (%v)", target, err)
	}
	if err := temporary.Chmod(mode); err != nil {
		discard()
		return fmt.Sprintf("%s: state could not be saved (%v)", target, err)
	}
	if err := temporary.Close(); err != nil {
		_ = os.Remove(name)
		return fmt.Sprintf("%s: state could not be saved (%v)", target, err)
	}
	if err := os.Rename(name, target); err != nil {
		_ = os.Remove(name)
		return fmt.Sprintf("%s: state could not be saved (%v)", target, err)
	}
	// Best effort: a filesystem that cannot flush a directory (some network
	// mounts) is not a reason to report a failure for a write that succeeded.
	if handle, err := os.Open(directory); err == nil {
		_ = handle.Sync()
		_ = handle.Close()
	}
	return ""
}
