package readers

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// The read-only guarantee, enforced structurally.
//
// gputop reads kernel state and never changes it (SPEC invariant: no code path
// opens a sysfs/procfs file for writing). A promise like that is worth exactly
// as much as the check behind it, so the check is here and it works on the
// parsed AST rather than on substrings: a comment or a doc string that merely
// *talks* about writing must not fail the build, and a write spelled in a way a
// grep does not anticipate must not slip past it.
//
// This is the Go counterpart of the reference's scan_for_writes: the same
// categories (filesystem mutators, write-capable opens, process launchers), the
// same refusal to condemn prose, and a self-test so the guarantee does not rest
// on a scanner that cannot fail.

// writeLeafNames are method names that mutate a file, matched on the final
// dotted segment so f.Write and w.WriteString are both caught regardless of the
// receiver's spelling. Pure in-memory writers (bytes.Buffer, strings.Builder)
// would also match, which is why the package-level test below is the real gate:
// readers must not write to *anything* but stderr.
var writeLeafNames = map[string]bool{
	"Write":       true,
	"WriteAt":     true,
	"WriteByte":   true,
	"WriteRune":   true,
	"WriteString": true,
	"WriteTo":     true,
	"Truncate":    true,
	"Chmod":       true,
	"Chown":       true,
	"Lchown":      true,
}

// writeFunctions are fully qualified writers, matched exactly. Their last
// segment ("Remove", "Link") is too ordinary to condemn on sight.
var writeFunctions = map[string]bool{
	"os.WriteFile":      true,
	"os.Create":         true,
	"os.CreateTemp":     true,
	"os.Mkdir":          true,
	"os.MkdirAll":       true,
	"os.MkdirTemp":      true,
	"os.Remove":         true,
	"os.RemoveAll":      true,
	"os.Rename":         true,
	"os.Link":           true,
	"os.Symlink":        true,
	"os.Chmod":          true,
	"os.Chown":          true,
	"os.Lchown":         true,
	"os.Truncate":       true,
	"os.Write":          true,
	"ioutil.WriteFile":  true,
	"ioutil.TempFile":   true,
	"ioutil.TempDir":    true,
	"syscall.Unlink":    true,
	"syscall.Unlinkat":  true,
	"syscall.Rename":    true,
	"syscall.Renameat":  true,
	"syscall.Mkdir":     true,
	"syscall.Mkdirat":   true,
	"syscall.Rmdir":     true,
	"syscall.Chmod":     true,
	"syscall.Fchmod":    true,
	"syscall.Creat":     true,
	"syscall.Truncate":  true,
	"syscall.Ftruncate": true,
}

// writeOpenFunctions are the openers whose *flags* decide whether they write.
var writeOpenFunctions = map[string]bool{
	"os.OpenFile":    true,
	"syscall.Open":   true,
	"syscall.Openat": true,
}

// writeFlagNames are the flag constants that ask for anything but reading.
var writeFlagNames = map[string]bool{
	"O_WRONLY": true,
	"O_RDWR":   true,
	"O_CREATE": true,
	"O_CREAT":  true,
	"O_TRUNC":  true,
	"O_APPEND": true,
}

// writeFlagBits is the same set as integers, for a flags argument written as a
// literal (O_WRONLY=1, O_RDWR=2, O_CREATE=64, O_TRUNC=512, O_APPEND=1024).
const writeFlagBits = 1 | 2 | 64 | 512 | 1024

// launcherFunctions start a process, which is the other way into a write: a
// shell can redirect to a path no Go call in this package ever names.
var launcherFunctions = map[string]bool{
	"exec.Command":        true,
	"exec.CommandContext": true,
	"os.StartProcess":     true,
	"syscall.Exec":        true,
	"syscall.ForkExec":    true,
}

// stdStreams are the only destinations fmt.Fprint* may target: writing to a
// console is not a write to kernel state.
var stdStreams = map[string]bool{
	"os.Stdout": true,
	"os.Stderr": true,
	"os.Stdin":  true,
}

// scanResult is what one source file was found to do.
type scanResult struct {
	Writes   []string
	Launches []string
}

// scanSource parses one Go source file and reports every write and every
// process launch, each as "line N: <what>". A parse error is returned rather
// than swallowed: a file the scanner cannot read is not a file it may call
// read-only.
func scanSource(name, source string) (scanResult, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, name, source, 0)
	if err != nil {
		return scanResult{}, err
	}
	var result scanResult
	line := func(pos token.Pos) int { return fset.Position(pos).Line }

	ast.Inspect(file, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		target := dottedName(call.Fun)
		leaf := target
		if index := strings.LastIndex(target, "."); index >= 0 {
			leaf = target[index+1:]
		}

		switch {
		case writeLeafNames[leaf] || writeFunctions[target]:
			result.Writes = append(result.Writes, describe(line(call.Pos()), target))
		case writeOpenFunctions[target]:
			if detail := openFlagsVerdict(call); detail != "" {
				result.Writes = append(result.Writes, describe(line(call.Pos()), detail))
			}
		case launcherFunctions[target]:
			result.Launches = append(result.Launches, describe(line(call.Pos()), target))
		case target == "fmt.Fprintf" || target == "fmt.Fprint" || target == "fmt.Fprintln":
			if destination := dottedName(firstArg(call)); !stdStreams[destination] {
				result.Writes = append(result.Writes,
					describe(line(call.Pos()), target+" to "+destinationOrUnknown(destination)))
			}
		}
		return true
	})
	return result, nil
}

// describe renders one finding.
func describe(line int, what string) string {
	return "line " + strconv.Itoa(line) + ": " + what
}

// destinationOrUnknown names a fmt.Fprint* target, or says the scanner could
// not name it. An unresolvable destination is a violation, not a pass: a flag
// this scanner cannot read is not one it is willing to call read-only.
func destinationOrUnknown(destination string) string {
	if destination == "" {
		return "<unresolvable>"
	}
	return destination
}

// firstArg returns a call's first argument, or nil.
func firstArg(call *ast.CallExpr) ast.Expr {
	if len(call.Args) == 0 {
		return nil
	}
	return call.Args[0]
}

// dottedName renders a call target as a dotted name, e.g. os.Remove or
// f.Write. Anything it cannot spell returns "".
func dottedName(expr ast.Expr) string {
	switch node := expr.(type) {
	case *ast.SelectorExpr:
		prefix := dottedName(node.X)
		if prefix == "" {
			return node.Sel.Name
		}
		return prefix + "." + node.Sel.Name
	case *ast.Ident:
		return node.Name
	case *ast.IndexExpr:
		// A generic instantiation, e.g. genericWriter[string].
		return dottedName(node.X)
	case *ast.ParenExpr:
		return dottedName(node.X)
	}
	return ""
}

// openFlagsVerdict inspects an opener's flags argument. It returns "" when the
// call is provably read-only, and a description of the problem otherwise.
func openFlagsVerdict(call *ast.CallExpr) string {
	if len(call.Args) < 2 {
		// os.OpenFile without flags does not compile; syscall.Open without
		// flags is read-only. Nothing to inspect.
		return ""
	}
	writing, known := classifyFlags(call.Args[1])
	switch {
	case writing:
		return dottedName(call.Fun) + " with write flags"
	case !known:
		return dottedName(call.Fun) + " with unreadable flags"
	default:
		return ""
	}
}

// classifyFlags resolves a flags expression into (writes, staticallyKnown).
// A flag it cannot read is reported as unknown rather than assumed innocent.
func classifyFlags(expr ast.Expr) (bool, bool) {
	switch node := expr.(type) {
	case *ast.BasicLit:
		if node.Kind != token.INT {
			return false, false
		}
		value, err := strconv.ParseInt(node.Value, 0, 64)
		if err != nil {
			return false, false
		}
		return value&writeFlagBits != 0, true
	case *ast.Ident:
		if node.Name == "O_RDONLY" {
			return false, true
		}
		if writeFlagNames[node.Name] {
			return true, true
		}
		return false, false
	case *ast.SelectorExpr:
		if node.Sel.Name == "O_RDONLY" {
			return false, true
		}
		if writeFlagNames[node.Sel.Name] {
			return true, true
		}
		return false, false
	case *ast.BinaryExpr:
		leftWrites, leftKnown := classifyFlags(node.X)
		rightWrites, rightKnown := classifyFlags(node.Y)
		if leftWrites || rightWrites {
			return true, true
		}
		return false, leftKnown && rightKnown
	case *ast.ParenExpr:
		return classifyFlags(node.X)
	}
	return false, false
}

// readersSources returns every non-test Go file in this package, keyed by name.
func readersSources(t *testing.T) map[string]string {
	t.Helper()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read package dir: %v", err)
	}
	sources := map[string]string{}
	for _, entry := range entries {
		name := entry.Name()
		if filepath.Ext(name) != ".go" || strings.HasSuffix(name, "_test.go") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(".", name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		sources[name] = string(data)
	}
	if len(sources) == 0 {
		t.Fatal("no non-test sources found; the scanner would pass vacuously")
	}
	return sources
}

// radeontopSource is the one file allowed to launch a process: the supervised
// radeontop child lives here and is owned by the sampler (see DESIGN.md). Any
// other reader that shells out is a new write path, not a convenience.
const radeontopSource = "radeontop.go"

// TestReadersNeverWriteToKernelState is the guarantee itself.
func TestReadersNeverWriteToKernelState(t *testing.T) {
	for name, source := range readersSources(t) {
		result, err := scanSource(name, source)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		if len(result.Writes) > 0 {
			t.Errorf("%s writes:\n\t%s", name, strings.Join(result.Writes, "\n\t"))
		}
	}
}

// TestReadersLaunchOnlyRadeontop bounds the other way into a write: a shell can
// redirect to a path no Go call names, so launchers are confined to one file.
func TestReadersLaunchOnlyRadeontop(t *testing.T) {
	for name, source := range readersSources(t) {
		result, err := scanSource(name, source)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		if len(result.Launches) > 0 && name != radeontopSource {
			t.Errorf("%s launches a process (only %s may):\n\t%s",
				name, radeontopSource, strings.Join(result.Launches, "\n\t"))
		}
	}
}

// scanSnippet parses one function body as a module and scans it.
func scanSnippet(t *testing.T, body string) scanResult {
	t.Helper()
	result, err := scanSource("snippet.go", "package p\n\n"+body+"\n")
	if err != nil {
		t.Fatalf("parse snippet %q: %v", body, err)
	}
	return result
}

// TestScanSourceCatchesWrites is the scanner's own test: a scanner that cannot
// fail is not a guarantee. Every snippet here is a real way to write to a file.
func TestScanSourceCatchesWrites(t *testing.T) {
	cases := map[string]string{
		"os.WriteFile":     `func f(p string) { _ = os.WriteFile(p, []byte("x"), 0o644) }`,
		"os.Create":        `func f(p string) { h, _ := os.Create(p); _ = h }`,
		"os.Remove":        `func f(p string) { _ = os.Remove(p) }`,
		"os.Rename":        `func f(p string) { _ = os.Rename(p, p) }`,
		"os.MkdirAll":      `func f(p string) { _ = os.MkdirAll(p, 0o755) }`,
		"os.Chmod":         `func f(p string) { _ = os.Chmod(p, 0o644) }`,
		"os.Symlink":       `func f(p string) { _ = os.Symlink(p, p) }`,
		"os.Truncate":      `func f(p string) { _ = os.Truncate(p, 0) }`,
		"File.Write":       `func f(h *os.File) { _, _ = h.Write([]byte("x")) }`,
		"File.WriteString": `func f(h *os.File) { _, _ = h.WriteString("x") }`,
		"File.Truncate":    `func f(h *os.File) { _ = h.Truncate(0) }`,
		"open O_WRONLY":    `func f(p string) { h, _ := os.OpenFile(p, os.O_WRONLY|os.O_CREATE, 0o644); _ = h }`,
		"open O_RDWR":      `func f(p string) { h, _ := os.OpenFile(p, os.O_RDWR, 0o644); _ = h }`,
		"open O_APPEND":    `func f(p string) { h, _ := os.OpenFile(p, os.O_APPEND, 0o644); _ = h }`,
		"open literal":     `func f(p string) { h, _ := os.OpenFile(p, 577, 0o644); _ = h }`,
		"open unknown":     `func f(p string, flags int) { h, _ := os.OpenFile(p, flags, 0o644); _ = h }`,
		"syscall.Open":     `func f(p string) { _, _ = syscall.Open(p, syscall.O_WRONLY, 0o644) }`,
		"fmt.Fprintf":      `func f(w io.Writer) { _, _ = fmt.Fprintf(w, "2") }`,
		"exec.Command":     `func f() { _ = exec.Command("sh", "-c", "echo 2 > /sys/x") }`,
		"exec.CommandCtx":  `func f(ctx context.Context) { _ = exec.CommandContext(ctx, "sh") }`,
		"os.StartProcess":  `func f() { _, _ = os.StartProcess("sh", nil, nil) }`,
	}
	for name, body := range cases {
		result := scanSnippet(t, body)
		if len(result.Writes) == 0 && len(result.Launches) == 0 {
			t.Errorf("%s: scanner found nothing in %q", name, body)
		}
	}
}

// TestScanSourceAllowsReadsAndProse pins the other half of the contract: a
// module that reads, and a module that only *talks* about writing, both pass.
// Condemning prose is what gets a scanner switched off, so it is tested.
func TestScanSourceAllowsReadsAndProse(t *testing.T) {
	cases := map[string]string{
		"os.ReadFile":     `func f(p string) { _, _ = os.ReadFile(p) }`,
		"os.Open":         `func f(p string) { h, _ := os.Open(p); _ = h }`,
		"os.ReadDir":      `func f(p string) { _, _ = os.ReadDir(p) }`,
		"open O_RDONLY":   `func f(p string) { h, _ := os.OpenFile(p, os.O_RDONLY, 0); _ = h }`,
		"open zero":       `func f(p string) { h, _ := os.OpenFile(p, 0, 0); _ = h }`,
		"stderr":          `func f() { _, _ = fmt.Fprintf(os.Stderr, "note: %s\n", "x") }`,
		"comment prose":   "// This reader never calls os.WriteFile, os.Create or os.Remove.\nfunc f() {}",
		"string prose":    `const doc = "no os.Create or os.WriteFile here"` + "\n\nfunc f() {}",
		"call in comment": "func f() {}\n\n// old := os.WriteFile(path, data, 0o644)\n",
	}
	for name, body := range cases {
		result := scanSnippet(t, body)
		if len(result.Writes) > 0 || len(result.Launches) > 0 {
			t.Errorf("%s: false positive %v / %v in %q",
				name, result.Writes, result.Launches, body)
		}
	}
}
