// Package check provides test assertion and snapshot helpers.
package check

import (
	"flag"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	. "github.com/typesanitizer/happygo/common/core"
	"github.com/typesanitizer/happygo/common/core/pathx"
)

func init() {
	// For compatibility with autogold and other packages that also define
	// an -update parameter, only register the flag if it's not already defined.
	// See NOTE(id: update-flag) — autogold uses the same pattern.
	if flag.Lookup("update") == nil {
		flag.Bool("update", false, "update snapshot files")
	}
}

// IsUpdateFlagSet reports whether the -update flag is set.
func IsUpdateFlagSet() bool {
	return flag.Lookup("update").Value.(flag.Getter).Get().(bool)
}

// Harness wraps testing.T with assertion and snapshot helpers.
type Harness struct {
	t *testing.T
}

// New returns a Harness. Value receiver per GO_STYLE_GUIDE.md.
func New(t *testing.T) Harness {
	t.Helper()
	return Harness{t: t}
}

// Parallel marks the test as parallel (wraps testing.T.Parallel).
func (h Harness) Parallel() {
	h.t.Helper()
	h.t.Parallel() //nolint:forbidigo // this is the wrapper
}

// Run runs a subtest with a Harness (wraps testing.T.Run).
func (h Harness) Run(name string, f func(Harness)) {
	h.t.Helper()
	h.t.Run(name, func(t *testing.T) { //nolint:forbidigo // this is the wrapper
		t.Helper()
		f(New(t))
	})
}

func (h Harness) fatalf(msg string, args ...any) {
	h.t.Helper()
	h.t.Fatalf(msg, args...) //nolint:forbidigo // this is the designated wrapper
}

// Assertf asserts that cond is true, failing the test if not.
func (h Harness) Assertf(cond bool, msg string, args ...any) {
	h.t.Helper()
	if !cond {
		h.fatalf(msg, args...)
	}
}

// NoErrorf asserts that err is nil, failing the test if not.
func (h Harness) NoErrorf(err error, msg string, args ...any) {
	h.t.Helper()
	if err != nil {
		h.fatalf("got error: %v\n"+msg, append([]any{err}, args...)...)
	}
}

// AssertPanicsWith asserts that f panics with want.
func (h Harness) AssertPanicsWith(want any, f func()) {
	h.t.Helper()
	var got any
	func() {
		defer func() {
			got = recover()
		}()
		f()
	}()
	h.Assertf(got != nil, "expected panic")
	AssertSame(h, want, got, "panic value")
}

// Logf logs a message via the underlying testing.T.
func (h Harness) Logf(msg string, args ...any) {
	h.t.Helper()
	h.t.Logf(msg, args...)
}

// AssertSame compares want and got using cmp.Diff and fails with a diff if they differ.
func AssertSame[T any](h Harness, want, got T, what string) {
	h.t.Helper()
	if diff := cmp.Diff(want, got); diff != "" {
		h.fatalf("%s mismatch (-want +got):\n%s", what, diff)
	}
}

// WriteTree creates files and directories under root from a map.
// Keys ending in "/" create directories (value must be "").
// Other keys create files with the value as content.
// All paths must be relative and stay within root.
func (h Harness) WriteTree(rootStr string, tree map[string]string) {
	h.t.Helper()
	root, err := pathx.ResolveAbsPath(rootStr)
	h.NoErrorf(err, "resolving absolute path for %q", rootStr)
	for path, content := range tree {
		h.Assertf(!filepath.IsAbs(path), "path %q must be relative to root %q", path, root.String())
		rel := NewRelPath(path)
		full := root.Join(rel)
		h.Assertf(full.MakeRelativeTo(root).IsSome(), "path %q escapes root %q", path, root.String())
		if strings.HasSuffix(path, "/") {
			h.Assertf(content == "", "directory path %q must have empty content", path)
			h.NoErrorf(full.MkdirAll(0o755), "creating directory %s", full.String())
			continue
		}
		h.NoErrorf(full.Dir().MkdirAll(0o755), "creating parent directory for %s", full.String())
		h.NoErrorf(full.WriteFile([]byte(content), 0o644), "writing file %s", full.String())
	}
}

// WriteFile writes a single file, creating parent directories as needed.
func (h Harness) WriteFile(path string, content string) {
	h.t.Helper()
	absPath, err := pathx.ResolveAbsPath(path)
	h.NoErrorf(err, "resolving absolute path for %q", path)
	dir, file := absPath.Split()
	h.WriteTree(dir.String(), map[string]string{file: content})
}

// InputPath keeps both user-provided and resolved path forms.
type InputPath struct {
	Input    string
	Resolved AbsPath
}

func NewInputPath(input string) (InputPath, error) {
	resolved, err := pathx.ResolveAbsPath(input)
	if err != nil {
		return InputPath{}, err
	}
	return InputPath{Input: input, Resolved: resolved}, nil
}

// Snapshot holds a path for file-based snapshot comparison.
type Snapshot struct {
	harness Harness
	path    InputPath
}

// SnapshotAt returns a Snapshot for the given file path.
// The path must resolve to a location inside the current working directory.
func (h Harness) SnapshotAt(path string) Snapshot {
	h.t.Helper()
	cwd, err := pathx.ResolveAbsPath(".")
	h.NoErrorf(err, "resolving working directory")
	snapshot, err := NewInputPath(path)
	h.NoErrorf(err, "resolving absolute path for %q", path)
	h.Assertf(snapshot.Resolved.MakeRelativeTo(cwd).IsSome(), "snapshot path %q escapes working directory", path)
	return Snapshot{harness: h, path: snapshot}
}

// Matches compares got to the snapshot file. If -update is set,
// the snapshot file is written (creating directories as needed).
func (s Snapshot) Matches(got string) {
	s.harness.t.Helper()

	if IsUpdateFlagSet() {
		s.harness.NoErrorf(s.path.Resolved.Dir().MkdirAll(0o755),
			"creating parent directory for snapshot %s", s.path.Resolved.String())
		s.harness.NoErrorf(s.path.Resolved.WriteFile([]byte(got), 0o644),
			"writing snapshot %s", s.path.Resolved.String())
		s.harness.Logf("updated snapshot: %s", s.path.Input)
		return
	}

	wantBytes, err := s.path.Resolved.ReadFile()
	s.harness.NoErrorf(err, "snapshot %s not found; run with -update to create it", s.path.Input)

	AssertSame(s.harness, string(wantBytes), got, "snapshot "+s.path.Input)
}
