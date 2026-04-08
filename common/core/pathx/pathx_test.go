package pathx_test

import (
	"path/filepath"
	"reflect"
	"runtime"
	"testing"

	"pgregory.net/rapid"

	"github.com/typesanitizer/happygo/common/assert"
	"github.com/typesanitizer/happygo/common/check"
	. "github.com/typesanitizer/happygo/common/check/prelude"
	"github.com/typesanitizer/happygo/common/core/pathx"
	"github.com/typesanitizer/happygo/common/core/pathx/pathx_testkit"
)

func TestMakeRelativeTo(t *testing.T) {
	h := check.New(t)

	h.Run("NeverPanics", func(h check.Harness) {
		h.Parallel()

		base := h.T().TempDir()
		segmentsGen := rapid.SliceOf(pathx_testkit.ComponentGen())
		rapid.Check(h.T(), func(t *rapid.T) {
			rootSegs := segmentsGen.Draw(t, "root_segments")
			pathSegs := segmentsGen.Draw(t, "path_segments")

			root := pathx.NewAbsPath(filepath.Join(append([]string{base}, rootSegs...)...))
			path := pathx.NewAbsPath(filepath.Join(append([]string{base}, pathSegs...)...))
			_ = path.MakeRelativeTo(root)
		})
	})

	h.Run("InsideRoot", func(h check.Harness) {
		h.Parallel()

		root := pathx.NewAbsPath(h.T().TempDir())
		inside := root.JoinComponents("a", "b")
		rel := inside.MakeRelativeTo(root)
		h.Assertf(rel.IsSome(), "inside path should be relative to root")
		check.AssertSame(h, inside.String(), rel.Unwrap().AsAbsPath().String(), "AsAbsPath()")
	})

	h.Run("OutsideRoot", func(h check.Harness) {
		h.Parallel()

		root := pathx.NewAbsPath(h.T().TempDir())
		outside := root.Join(pathx.NewRelPath(filepath.Join("..", "outside")))
		h.Assertf(!outside.MakeRelativeTo(root).IsSome(), "outside path should not be relative to root")
	})

	h.Run("ContainedPathRoundTrip", func(h check.Harness) {
		h.Parallel()

		root := pathx.NewAbsPath(h.T().TempDir())
		safeRelGen := pathx_testkit.SafeRelPathGen()
		rapid.Check(h.T(), func(t *rapid.T) {
			basic := check.NewBasic(t)
			rel := safeRelGen.Draw(t, "safe_rel")
			basic.Assertf(root.LexicallyContains(rel),
				"safe relative path unexpectedly escaped root: %q", rel.String())

			child := root.Join(rel)
			relPath := child.MakeRelativeTo(root)
			basic.Assertf(relPath.IsSome(),
				"MakeRelativeTo(root) unexpectedly returned None for child %q", child.String())

			resolved := relPath.Unwrap().AsAbsPath()
			check.AssertSame(basic, child.String(), resolved.String(), "AsAbsPath()")
		})
	})

	h.Run("RejectsEscapingPaths", func(h check.Harness) {
		h.Parallel()

		root := pathx.NewAbsPath(h.T().TempDir())
		escapingRelGen := pathx_testkit.EscapingRelPathGen()
		rapid.Check(h.T(), func(t *rapid.T) {
			basic := check.NewBasic(t)
			rel := escapingRelGen.Draw(t, "escaping_rel")
			basic.Assertf(!root.LexicallyContains(rel),
				"escaping path unexpectedly contained: %q", rel.String())

			child := root.Join(rel)
			basic.Assertf(!child.MakeRelativeTo(root).IsSome(),
				"MakeRelativeTo(root) unexpectedly returned Some for escaping child %q", child.String())
		})
	})
}

func TestSplit(t *testing.T) {
	h := check.New(t)

	h.Run("RoundTrip", func(h check.Harness) {
		h.Parallel()

		root := pathx.NewAbsPath(h.T().TempDir())
		componentsGen := rapid.SliceOfN(pathx_testkit.ComponentGen(), 1, 6)
		rapid.Check(h.T(), func(t *rapid.T) {
			basic := check.NewBasic(t)
			components := componentsGen.Draw(t, "components")
			path := root.JoinComponents(components...)
			dir, file := path.Split()
			check.AssertSame(basic, components[len(components)-1], file, "Split() file")

			reconstructed := dir.Join(pathx.NewRelPath(file))
			check.AssertSame(basic, path.String(), reconstructed.String(), "Split() round-trip")
		})
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
	check.AssertSame(h, want.String(), got.String(), "Join vs JoinComponents")
}

func TestRootRelPathBasics(t *testing.T) {
	h := check.New(t)
	root := pathx.NewAbsPath(t.TempDir())
	rel := pathx.NewRelPath(filepath.Join("dir", "file.txt"))
	rootRelPath := pathx.NewRootRelPath(root, rel)
	check.AssertSame(h, rel.String(), rootRelPath.String(), "RootRelPath.String()")
	check.AssertSame(h, root.Join(rel).String(), rootRelPath.AsAbsPath().String(), "RootRelPath.AsAbsPath()")
}

func TestResolveAbsPath(t *testing.T) {
	h := check.New(t)
	_ = Do(pathx.ResolveAbsPath("."))(h)
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

	patterns := []string{"a/b"}
	if runtime.GOOS == "windows" {
		patterns = append(patterns, `a\b`)
	}
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
