// Package check provides test assertion and snapshot helpers.
package check

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/typesanitizer/happygo/common/internal/pathx"
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

// Snapshot holds a path for file-based snapshot comparison.
type Snapshot struct {
	harness Harness
	path    string
}

// SnapshotAt returns a Snapshot for the given file path.
// The path must resolve to a location inside the current working directory.
func (h Harness) SnapshotAt(path string) Snapshot {
	h.t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		h.fatalf("getting working directory: %v", err)
	}
	h.Assertf(pathx.LexicallyContains(cwd, path), "snapshot path %q escapes working directory", path)
	return Snapshot{harness: h, path: path}
}

// Matches compares got to the snapshot file. If -update is set,
// the snapshot file is written (creating directories as needed).
func (s Snapshot) Matches(got string) {
	s.harness.t.Helper()

	if IsUpdateFlagSet() {
		s.harness.WriteFile(s.path, got)
		s.harness.Logf("updated snapshot: %s", s.path)
		return
	}

	wantBytes, err := os.ReadFile(s.path)
	s.harness.NoErrorf(err, "snapshot %s not found; run with -update to create it", s.path)

	AssertSame(s.harness, string(wantBytes), got, "snapshot "+s.path)
}
