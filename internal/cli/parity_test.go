package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gputop/gputop-go/internal/config"
	"github.com/gputop/gputop-go/internal/sampler"
)

// This file is the Phase 1 parity gate: it proves that --dump on a synthetic
// tree agrees with the Python reference's --dump *field for field*.
//
// The tree and the golden are committed under testdata/parity/ and are produced
// by scripts/parity_fixture.py, which builds the tree with the reference's own
// fixture helpers and runs the reference CLI over it. Regenerate them with:
//
//	/path/to/radeon-btop-py/.venv2/bin/python scripts/parity_fixture.py
//
// The comparison is structural, never textual: Go's encoding/json writes a
// float as `25` where Python writes `25.0`, so raw bytes can never be equal.
// Key *order* is still compared, because the two are deliberately kept aligned
// (struct declaration order == the reference's dict insertion order) and an
// order drift would silently break a textual diff downstream.
//
// A handful of fields are properties of the machine that ran each side rather
// than of the readers, so they are normalised away and named here so the
// exclusion stays visible rather than silent:
//
//   - timestamp_ns and the sampler's durations: wall-clock measurements.
//   - processes[].engines[].{delta_ns,window_ns,percent} and the derived
//     processes[].engine_percent: deltas over a real (and therefore different)
//     sampling window; the cumulative total_ns beside them is deterministic and
//     is compared.
//   - processes[].user: resolved through the host's user database, which the
//     container does not share with the host that generated the golden.

const parityDir = "testdata/parity"

// TestDumpParityWithReference is the gate the phase plan names: the Go dump and
// the Python dump, over one synthetic tree, agree on every field that is a
// function of that tree.
func TestDumpParityWithReference(t *testing.T) {
	golden := parseJSONDocument(t, mustRead(t, filepath.Join(parityDir, "dump.json")))

	cfg := config.Load(filepath.Join(parityDir, "gputop.yaml"))
	if len(cfg.Warnings) != 0 {
		t.Fatalf("fixture config produced warnings: %v", cfg.Warnings)
	}
	in := OneShot{
		Config: &cfg,
		Sampler: sampler.SamplerOptions{
			IntervalS:        sampler.MinIntervalS,
			CollectProcesses: true,
			DrmRoot:          filepath.Join(parityDir, "tree", "sys", "class", "drm"),
			ProcRoot:         filepath.Join(parityDir, "tree", "proc"),
			// PciRoot is left at its default on purpose: the reference does not
			// take one either, and the fixture BDF cannot exist on real
			// hardware, so both sides deterministically report boot_vga false.
		},
	}

	var out bytes.Buffer
	status, err := Dump(&out, in)
	if err != nil {
		t.Fatalf("Dump: %v", err)
	}
	if status != 0 {
		t.Fatalf("Dump status = %d, want 0; output:\n%s", status, out.String())
	}

	got := parseJSONDocument(t, out.Bytes())
	if diff := compareJSON("", golden, got); diff != "" {
		t.Fatalf("Go --dump differs from the reference:\n%s", diff)
	}
}

// mustRead reads a fixture file or fails the test.
func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return data
}

// jsonKind names the JSON value kinds the comparator distinguishes. It is
// deliberately finer-grained than the Go types: a null and a missing key are
// different claims, and so are the number 0 and the string "0".
type jsonKind string

const (
	jsonNull   jsonKind = "null"
	jsonBool   jsonKind = "bool"
	jsonNumber jsonKind = "number"
	jsonString jsonKind = "string"
	jsonArray  jsonKind = "array"
	jsonObject jsonKind = "object"
)

// jsonNode is one parsed JSON value. Objects keep their keys in document order
// so the gate can compare key order as well as values, which is what makes the
// "declaration order matches the reference" claim testable.
type jsonNode struct {
	kind    jsonKind
	str     string
	num     float64
	boolean bool
	keys    []string
	fields  []*jsonNode
	items   []*jsonNode
}

// parseJSONDocument decodes a document into the order-preserving tree.
func parseJSONDocument(t *testing.T, data []byte) *jsonNode {
	t.Helper()
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	node, err := decodeJSONValue(dec)
	if err != nil {
		t.Fatalf("parse JSON: %v", err)
	}
	return node
}

// decodeJSONValue reads exactly one JSON value, recursing into containers.
func decodeJSONValue(dec *json.Decoder) (*jsonNode, error) {
	token, err := dec.Token()
	if err != nil {
		return nil, err
	}
	switch value := token.(type) {
	case json.Delim:
		switch value {
		case '{':
			return decodeJSONObject(dec)
		case '[':
			return decodeJSONArray(dec)
		}
		return nil, fmt.Errorf("unexpected delimiter %q", value)
	case bool:
		return &jsonNode{kind: jsonBool, boolean: value}, nil
	case json.Number:
		number, err := value.Float64()
		if err != nil {
			return nil, err
		}
		return &jsonNode{kind: jsonNumber, num: number}, nil
	case string:
		return &jsonNode{kind: jsonString, str: value}, nil
	case nil:
		return &jsonNode{kind: jsonNull}, nil
	}
	return nil, fmt.Errorf("unexpected token %T", token)
}

// decodeJSONObject reads a `{...}` body, keeping key order.
func decodeJSONObject(dec *json.Decoder) (*jsonNode, error) {
	node := &jsonNode{kind: jsonObject}
	for dec.More() {
		keyToken, err := dec.Token()
		if err != nil {
			return nil, err
		}
		key, ok := keyToken.(string)
		if !ok {
			return nil, fmt.Errorf("object key is %T", keyToken)
		}
		child, err := decodeJSONValue(dec)
		if err != nil {
			return nil, err
		}
		node.keys = append(node.keys, key)
		node.fields = append(node.fields, child)
	}
	_, err := dec.Token() // consume '}'
	return node, err
}

// decodeJSONArray reads a `[...]` body.
func decodeJSONArray(dec *json.Decoder) (*jsonNode, error) {
	node := &jsonNode{kind: jsonArray}
	for dec.More() {
		child, err := decodeJSONValue(dec)
		if err != nil {
			return nil, err
		}
		node.items = append(node.items, child)
	}
	_, err := dec.Token() // consume ']'
	return node, err
}

// volatilePaths are the field paths whose value is a property of the machine
// that produced the dump rather than of the readers. Array indices are written
// as `[]`; the comparator normalises real indices to that spelling before
// looking a path up. See the file comment for why each one is here.
var volatilePaths = map[string]bool{
	"timestamp_ns":                    true,
	"stats.last_duration_ms":          true,
	"stats.avg_duration_ms":           true,
	"processes[].user":                true,
	"processes[].engine_percent":      true,
	"processes[].engines[].delta_ns":  true,
	"processes[].engines[].window_ns": true,
	"processes[].engines[].percent":   true,
}

// compareJSON returns a description of the first difference between two parsed
// documents, or "" when they agree. It walks in document order so the reported
// path is the first divergence a reader would hit.
func compareJSON(path string, want, got *jsonNode) string {
	if isVolatile(path) {
		return ""
	}
	if want.kind != got.kind {
		return fmt.Sprintf("%s: kind %s, want %s", displayPath(path), got.kind, want.kind)
	}
	switch want.kind {
	case jsonObject:
		if diff := compareKeys(path, want.keys, got.keys); diff != "" {
			return diff
		}
		for i, key := range want.keys {
			if diff := compareJSON(childPath(path, key), want.fields[i], got.fields[i]); diff != "" {
				return diff
			}
		}
	case jsonArray:
		if len(want.items) != len(got.items) {
			return fmt.Sprintf("%s: length %d, want %d", displayPath(path), len(got.items), len(want.items))
		}
		for i := range want.items {
			if diff := compareJSON(fmt.Sprintf("%s[%d]", path, i), want.items[i], got.items[i]); diff != "" {
				return diff
			}
		}
	case jsonNumber:
		if !nearlyEqual(want.num, got.num) {
			return fmt.Sprintf("%s: %v, want %v", displayPath(path), got.num, want.num)
		}
	case jsonString:
		if want.str != got.str {
			return fmt.Sprintf("%s: %q, want %q", displayPath(path), got.str, want.str)
		}
	case jsonBool:
		if want.boolean != got.boolean {
			return fmt.Sprintf("%s: %t, want %t", displayPath(path), got.boolean, want.boolean)
		}
	}
	return ""
}

// compareKeys reports the first key that is missing, extra or out of order.
// Object key order is part of the contract, so it is compared, not ignored.
func compareKeys(path string, want, got []string) string {
	for i := range want {
		if i >= len(got) {
			return fmt.Sprintf("%s: missing key %q", displayPath(path), want[i])
		}
		if got[i] != want[i] {
			return fmt.Sprintf("%s: key %d is %q, want %q (keys: %v vs %v)",
				displayPath(path), i, got[i], want[i], got, want)
		}
	}
	if len(got) > len(want) {
		return fmt.Sprintf("%s: extra key %q", displayPath(path), got[len(want)])
	}
	return ""
}

// childPath extends a path with an object key, joining the root cleanly.
func childPath(path, key string) string {
	if path == "" {
		return key
	}
	return path + "." + key
}

// displayPath renders the root as `<root>` so a failure at the top level still
// reads as a location.
func displayPath(path string) string {
	if path == "" {
		return "<root>"
	}
	return path
}

// isVolatile reports whether a field path is normalised away.
func isVolatile(path string) bool {
	if path == "" {
		return false
	}
	return volatilePaths[normalisePath(path)]
}

// normalisePath rewrites every array index to `[]` so `processes[0].user` and
// `processes[3].user` are the same normalised path.
func normalisePath(path string) string {
	var out strings.Builder
	for i := 0; i < len(path); i++ {
		if path[i] != '[' {
			out.WriteByte(path[i])
			continue
		}
		out.WriteString("[]")
		for i < len(path) && path[i] != ']' {
			i++
		}
	}
	return out.String()
}

// nearlyEqual compares two floats exactly, then with a relative tolerance.
//
// The two implementations run the same formulas in different languages, and a
// division that rounds the same way in both is not something to assume: a
// percentage such as 81.29675810473816 can land one unit in the last place away
// from 81.29675810473815. The tolerance is deliberately far tighter than any
// real disagreement -- a wrong unit, scale or formula moves the value by orders
// of magnitude -- so it separates float rounding from a bug without hiding one.
func nearlyEqual(want, got float64) bool {
	if want == got {
		return true
	}
	scale := math.Max(math.Abs(want), math.Abs(got))
	return math.Abs(want-got) <= 1e-9*scale
}
