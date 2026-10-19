package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goccy/go-yaml"
)

// stateFile seeds a config at path and points the state resolution at it.
func stateFile(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if contents != "" {
		if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	t.Setenv(StateEnv, path)
	return path
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

func sampleState() StateConfig {
	theme, interval, sort := "dracula", 2000, "vram"
	reverse := true
	return StateConfig{
		Theme: &theme, IntervalMs: &interval, Sort: &sort, SortReverse: &reverse,
		Filter: "firefox", ShowIdle: true, ProcessTree: true,
		FullCommand: true, Overview: false, GpuIndex: 1,
	}
}

func TestSaveStateKeepsTheRestOfTheDocumentAndItsComments(t *testing.T) {
	path := stateFile(t, `# my config
interval_ms: 500   # fast

gpu:
  # the name I chose
  names:
    "0000:0c:00.0": "Big GPU"

state:
  theme: default
  process_sort: total
`)
	if msg := SaveState(sampleState()); msg != "" {
		t.Fatalf("SaveState = %q, want success", msg)
	}
	got := readFile(t, path)
	for _, want := range []string{
		"# my config", "# fast", "# the name I chose",
		"names:", "Big GPU", "interval_ms: 500",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("saved document lost %q:\n%s", want, got)
		}
	}
	for _, want := range []string{
		"theme: dracula", "interval_ms: 2000", "process_sort: vram",
		"process_sort_reverse: true", "process_filter: firefox",
		"show_idle_processes: true", "show_process_tree: true",
		"full_command: true", "overview: false", "gpu_index: 1",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("saved document missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "process_sort: total") {
		t.Errorf("the old state value survived:\n%s", got)
	}
}

func TestSaveStateAddsTheSectionWhenThereIsNone(t *testing.T) {
	path := stateFile(t, `# hand-written
gpu:
  kind: dgpu
`)
	if msg := SaveState(sampleState()); msg != "" {
		t.Fatalf("SaveState = %q, want success", msg)
	}
	got := readFile(t, path)
	if !strings.Contains(got, "# hand-written") || !strings.Contains(got, "kind: dgpu") {
		t.Fatalf("saved document lost the user's keys:\n%s", got)
	}
	if !strings.Contains(got, "state:\n") {
		t.Fatalf("saved document has no state block:\n%s", got)
	}
	if !strings.Contains(got, "\n  theme: dracula\n") {
		t.Fatalf("state block is not nested at two spaces:\n%s", got)
	}
	var raw map[string]any
	if err := yaml.Unmarshal([]byte(got), &raw); err != nil {
		t.Fatalf("saved document does not parse: %v\n%s", err, got)
	}
	if _, ok := raw["state"].(map[string]any); !ok {
		t.Fatalf("state is not a mapping:\n%s", got)
	}
}

func TestSaveStateRoundTripsThroughTheLoader(t *testing.T) {
	stateFile(t, "")
	if msg := SaveState(sampleState()); msg != "" {
		t.Fatalf("SaveState = %q, want success", msg)
	}
	var warnings []string
	loaded := loadSavedState(&warnings)
	if loaded == nil {
		t.Fatalf("loadSavedState returned nil; warnings %v", warnings)
	}
	if loaded.Theme == nil || *loaded.Theme != "dracula" {
		t.Errorf("theme = %v, want dracula", loaded.Theme)
	}
	if loaded.IntervalMs == nil || *loaded.IntervalMs != 2000 {
		t.Errorf("interval = %v, want 2000", loaded.IntervalMs)
	}
	if loaded.Sort == nil || *loaded.Sort != "vram" {
		t.Errorf("sort = %v, want vram", loaded.Sort)
	}
	if loaded.SortReverse == nil || !*loaded.SortReverse {
		t.Errorf("sort_reverse = %v, want true", loaded.SortReverse)
	}
	if loaded.Filter != "firefox" || !loaded.ShowIdle || !loaded.ProcessTree ||
		!loaded.FullCommand || loaded.Overview || loaded.GpuIndex != 1 {
		t.Errorf("state did not round-trip: %+v", *loaded)
	}
}

func TestSaveStateOmitsUnrecordedPointers(t *testing.T) {
	path := stateFile(t, "")
	if msg := SaveState(StateConfig{Filter: "x"}); msg != "" {
		t.Fatalf("SaveState = %q, want success", msg)
	}
	got := readFile(t, path)
	for _, unwanted := range []string{"theme:", "interval_ms:", "process_sort:", "process_sort_reverse:"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("unrecorded field was written: %q\n%s", unwanted, got)
		}
	}
}

func TestSaveStateRefusesToRewriteAnUnparseableFile(t *testing.T) {
	original := "gpu: [unclosed\n  names: {oops\n"
	path := stateFile(t, original)
	msg := SaveState(sampleState())
	if msg == "" {
		t.Fatal("SaveState reported success on an unparseable file")
	}
	if !strings.Contains(msg, "not valid YAML") {
		t.Errorf("message = %q, want it to name the parse failure", msg)
	}
	if got := readFile(t, path); got != original {
		t.Fatalf("the unparseable file was modified:\n%s", got)
	}
}

func TestSaveStateCreatesAPrivateFile(t *testing.T) {
	path := stateFile(t, "")
	if msg := SaveState(sampleState()); msg != "" {
		t.Fatalf("SaveState = %q", msg)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("mode = %o, want 600", perm)
	}
}

func TestSaveStateKeepsTheExistingPermissions(t *testing.T) {
	path := stateFile(t, "interval_ms: 500\n")
	if err := os.Chmod(path, 0o640); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	if msg := SaveState(sampleState()); msg != "" {
		t.Fatalf("SaveState = %q", msg)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o640 {
		t.Errorf("mode = %o, want the file's own 640", perm)
	}
}

func TestSaveStateLeavesNoTemporaryFileBehind(t *testing.T) {
	path := stateFile(t, "interval_ms: 500\n")
	if msg := SaveState(sampleState()); msg != "" {
		t.Fatalf("SaveState = %q", msg)
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".tmp") {
			t.Errorf("temporary file left behind: %s", entry.Name())
		}
	}
}
