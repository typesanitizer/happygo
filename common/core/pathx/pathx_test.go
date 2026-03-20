package pathx_test

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"pgregory.net/rapid"

	"github.com/typesanitizer/happygo/common/assert"
	"github.com/typesanitizer/happygo/common/check"
	"github.com/typesanitizer/happygo/common/core/pathx"
	"github.com/typesanitizer/happygo/common/core/pathx/internal/winpath"
)

func pathTokenGen() *rapid.Generator[string] {
	return rapid.StringMatching(`[A-Za-z0-9][A-Za-z0-9._-]{0,7}`)
}

func safeRelPathGen() *rapid.Generator[pathx.RelPath] {
	return rapid.Custom(func(t *rapid.T) pathx.RelPath {
		componentCount := rapid.IntRange(0, 12).Draw(t, "component_count")
		components := make([]string, 0, componentCount)
		depth := 0
		for range componentCount {
			kind := rapid.IntRange(0, 3).Draw(t, "kind")
			switch kind {
			case 0:
				components = append(components, ".")
			case 1:
				if depth > 0 && rapid.Bool().Draw(t, "go_up") {
					components = append(components, "..")
					depth--
					continue
				}
				fallthrough
			default:
				components = append(components, pathTokenGen().Draw(t, "token"))
				depth++
			}
		}
		if len(components) == 0 {
			return pathx.NewRelPath(".")
		}
		return pathx.NewRelPath(strings.Join(components, string(filepath.Separator)))
	})
}

func escapingRelPathGen() *rapid.Generator[pathx.RelPath] {
	return rapid.Custom(func(t *rapid.T) pathx.RelPath {
		upCount := rapid.IntRange(1, 6).Draw(t, "up_count")
		tailCount := rapid.IntRange(0, 6).Draw(t, "tail_count")
		components := make([]string, 0, upCount+tailCount)
		for range upCount {
			components = append(components, "..")
		}
		for range tailCount {
			components = append(components, pathTokenGen().Draw(t, "tail"))
		}
		return pathx.NewRelPath(strings.Join(components, string(filepath.Separator)))
	})
}

func TestMakeRelativeToNeverPanics(t *testing.T) {
	base := t.TempDir()
	segmentsGen := rapid.SliceOf(pathTokenGen())

	rapid.Check(t, func(t *rapid.T) {
		rootSegs := segmentsGen.Draw(t, "root_segments")
		pathSegs := segmentsGen.Draw(t, "path_segments")

		root := pathx.NewAbsPath(filepath.Join(append([]string{base}, rootSegs...)...))
		path := pathx.NewAbsPath(filepath.Join(append([]string{base}, pathSegs...)...))
		_ = path.MakeRelativeTo(root)
	})
}

func TestRootRelPathResolveRoundTripRapid(t *testing.T) {
	root := pathx.NewAbsPath(t.TempDir())
	safeRelGen := safeRelPathGen()

	rapid.Check(t, func(t *rapid.T) {
		rel := safeRelGen.Draw(t, "safe_rel")
		if !root.LexicallyContains(rel) {
			t.Fatalf("safe relative path unexpectedly escaped root: %q", rel.String())
		}

		child := root.Join(rel)
		relOpt := child.MakeRelativeTo(root)
		if !relOpt.IsSome() {
			t.Fatalf("MakeRelativeTo(root) unexpectedly returned None for child %q", child.String())
		}

		rootRelPath, _ := relOpt.Get()
		resolved := rootRelPath.Resolve()
		if resolved.String() != child.String() {
			t.Fatalf("Resolve() = %q, want %q", resolved.String(), child.String())
		}
	})
}

func TestEscapingRelPathRejectedRapid(t *testing.T) {
	root := pathx.NewAbsPath(t.TempDir())
	escapingRelGen := escapingRelPathGen()

	rapid.Check(t, func(t *rapid.T) {
		rel := escapingRelGen.Draw(t, "escaping_rel")
		if root.LexicallyContains(rel) {
			t.Fatalf("escaping path unexpectedly contained: %q", rel.String())
		}

		child := root.Join(rel)
		if child.MakeRelativeTo(root).IsSome() {
			t.Fatalf("MakeRelativeTo(root) unexpectedly returned Some for escaping child %q", child.String())
		}
	})
}

func TestSplitRoundTripRapid(t *testing.T) {
	root := pathx.NewAbsPath(t.TempDir())
	componentsGen := rapid.SliceOfN(pathTokenGen(), 1, 6)

	rapid.Check(t, func(t *rapid.T) {
		components := componentsGen.Draw(t, "components")
		path := root.JoinComponents(components...)
		dir, file := path.Split()
		reconstructed := dir.Join(pathx.NewRelPath(file))
		if reconstructed.String() != path.String() {
			t.Fatalf("Split round-trip = %q, want %q", reconstructed.String(), path.String())
		}
	})
}

func TestRelPathComponents(t *testing.T) {
	h := check.New(t)

	tests := []struct {
		path string
		want []string
	}{
		{path: "a", want: []string{"a"}},
		{path: "a/b/c", want: []string{"a", "b", "c"}},
		{path: "a//b///c", want: []string{"a", "b", "c"}},
		{path: "a/./../b", want: []string{"a", ".", "..", "b"}},
	}

	for _, tt := range tests {
		tt := tt
		h.Run(tt.path, func(h check.Harness) {
			got := make([]string, 0)
			for c := range pathx.NewRelPath(tt.path).Components() {
				got = append(got, c)
			}
			h.Assertf(reflect.DeepEqual(got, tt.want), "Components(%q) = %#v, want %#v", tt.path, got, tt.want)
		})
	}
}

func TestLexicallyContains(t *testing.T) {
	h := check.New(t)
	root := pathx.NewAbsPath(t.TempDir())

	tests := []struct {
		path string
		want bool
	}{
		{path: ".", want: true},
		{path: "a/b", want: true},
		{path: "a/../b", want: true},
		{path: "../b", want: false},
		{path: "a/../../b", want: false},
	}

	for _, tt := range tests {
		tt := tt
		h.Run(tt.path, func(h check.Harness) {
			got := root.LexicallyContains(pathx.NewRelPath(tt.path))
			h.Assertf(got == tt.want, "LexicallyContains(%q) = %v, want %v", tt.path, got, tt.want)
		})
	}
}

func TestJoinMatchesJoinComponents(t *testing.T) {
	h := check.New(t)
	root := pathx.NewAbsPath(t.TempDir())
	rel := pathx.NewRelPath(filepath.Join("a", "b", "c"))

	got := root.Join(rel)
	want := root.JoinComponents("a", "b", "c")
	h.Assertf(got.String() == want.String(), "Join(rel) = %q, want %q", got.String(), want.String())
}

func TestSplitRoundTrip(t *testing.T) {
	h := check.New(t)
	path := pathx.NewAbsPath(t.TempDir()).JoinComponents("dir", "file.txt")
	dir, file := path.Split()
	h.Assertf(file == "file.txt", "Split() file = %q, want %q", file, "file.txt")
	got := dir.Join(pathx.NewRelPath(file))
	h.Assertf(got.String() == path.String(), "Split() round-trip = %q, want %q", got.String(), path.String())
}

func TestMakeRelativeToInsideAndOutside(t *testing.T) {
	h := check.New(t)
	root := pathx.NewAbsPath(t.TempDir())
	inside := root.JoinComponents("a", "b")
	h.Assertf(inside.MakeRelativeTo(root).IsSome(), "inside path should be relative to root")

	outside := root.Join(pathx.NewRelPath(filepath.Join("..", "outside")))
	h.Assertf(!outside.MakeRelativeTo(root).IsSome(), "outside path should not be relative to root")
}

func TestRootRelPathBasics(t *testing.T) {
	h := check.New(t)
	root := pathx.NewAbsPath(t.TempDir())
	rel := pathx.NewRelPath(filepath.Join("dir", "file.txt"))
	rootRelPath := pathx.NewRootRelPath(root, rel)
	h.Assertf(rootRelPath.String() == rel.String(), "RootRelPath.String() = %q, want %q", rootRelPath.String(), rel.String())
	h.Assertf(rootRelPath.Resolve().String() == root.Join(rel).String(), "RootRelPath.Resolve() mismatch")
}

func TestResolveAbsPath(t *testing.T) {
	h := check.New(t)
	got, err := pathx.ResolveAbsPath(".")
	h.NoErrorf(err, "ResolveAbsPath(.)")
	h.Assertf(filepath.IsAbs(got.String()), "ResolveAbsPath(.) = %q, want absolute path", got.String())
}

func TestRejectsEmptyPaths(t *testing.T) {
	h := check.New(t)
	want := assert.AssertionError{Fmt: "precondition violation: path is empty", Args: nil}

	tests := []struct {
		name string
		call func()
	}{
		{name: "NewAbsPath", call: func() { _ = pathx.NewAbsPath("") }},
		{name: "NewRelPath", call: func() { _ = pathx.NewRelPath("") }},
		{name: "ResolveAbsPath", call: func() { _, _ = pathx.ResolveAbsPath("") }},
	}

	for _, tt := range tests {
		tt := tt
		h.Run(tt.name, func(h check.Harness) {
			h.AssertPanicsWith(want, tt.call)
		})
	}
}

func TestMkdirTempRejectsPathSeparatorInPattern(t *testing.T) {
	h := check.New(t)
	root := pathx.NewAbsPath(t.TempDir())

	patterns := []string{"a/b", `a\\b`}
	for _, pattern := range patterns {
		pattern := pattern
		h.Run(pattern, func(h check.Harness) {
			want := assert.AssertionError{
				Fmt:  "precondition violation: pattern contains path separator: %q",
				Args: []any{pattern},
			}
			h.AssertPanicsWith(want, func() {
				_, _ = root.MkdirTemp(pattern)
			})
		})
	}
}

func TestIsWindowsStyleAbsPath(t *testing.T) {
	h := check.New(t)

	tests := []struct {
		path string
		want bool
	}{
		{path: `C:\Windows\System32`, want: true},
		{path: `C:/Windows/System32`, want: true},
		{path: `C:relative`, want: true},
		{path: `\\server\share\dir`, want: true},
		{path: `\Windows\System32`, want: true},
		{path: `//server/share/dir`, want: true},
		{path: `Å:\Windows\System32`, want: false},
		{path: `1:\Windows\System32`, want: false},
		{path: `/usr/local/bin`, want: false},
		{path: `relative/path`, want: false},
	}

	for _, tt := range tests {
		tt := tt
		h.Run(tt.path, func(h check.Harness) {
			got := winpath.IsWindowsStyleAbsPath(tt.path)
			h.Assertf(got == tt.want, "IsWindowsStyleAbsPath(%q) = %v, want %v", tt.path, got, tt.want)
		})
	}
}
