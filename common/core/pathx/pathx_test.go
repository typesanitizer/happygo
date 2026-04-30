package pathx_test

import (
	"fmt"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"pgregory.net/rapid"

	"github.com/typesanitizer/happygo/common/assert"
	"github.com/typesanitizer/happygo/common/check"
	"github.com/typesanitizer/happygo/common/core/option"
	"github.com/typesanitizer/happygo/common/core/pathx"
	"github.com/typesanitizer/happygo/common/core/pathx/pathx_testkit"
	"github.com/typesanitizer/happygo/common/errorx"
	"github.com/typesanitizer/happygo/common/iterx"
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
		{path: "a/./../b", want: []string{"b"}},
	}

	for _, tt := range tests {
		h.Run(tt.path, func(h check.Harness) {
			got := iterx.Collect(pathx.NewRelPath(tt.path).Components())
			check.AssertSame(h, tt.want, got, fmt.Sprintf("Components(%q)", tt.path))
		})
	}
}

func TestRelPathAncestors(t *testing.T) {
	h := check.New(t)

	tests := []struct {
		path string
		want []string
	}{
		{path: ".", want: nil},
		{path: "a", want: nil},
		{path: "a/b", want: []string{"a"}},
		{path: "a/b/c", want: []string{"a", filepath.Join("a", "b")}},
		{path: "a/b/c/d", want: []string{"a", filepath.Join("a", "b"), filepath.Join("a", "b", "c")}},
	}

	for _, tt := range tests {
		h.Run(tt.path, func(h check.Harness) {
			got := iterx.Collect(iterx.Map(pathx.NewRelPath(tt.path).Ancestors(), pathx.RelPath.String))
			check.AssertSame(h, tt.want, got, fmt.Sprintf("Ancestors(%q)", tt.path))
		})
	}
}

func TestRelPathRelativeTo(t *testing.T) {
	h := check.New(t)

	tests := []struct {
		path string
		base string
		want string
	}{
		{path: "a", base: ".", want: "a"},
		{path: "a/b/c", base: ".", want: filepath.Join("a", "b", "c")},
		{path: "a", base: "a", want: "."},
		{path: "a/b/c", base: "a/b/c", want: "."},
		{path: "a/b", base: "a", want: "b"},
		{path: "a/b/c", base: "a", want: filepath.Join("b", "c")},
		{path: "a/b/c", base: "a/b", want: "c"},
	}

	for _, tt := range tests {
		h.Run(fmt.Sprintf("%s_from_%s", tt.path, tt.base), func(h check.Harness) {
			got := pathx.NewRelPath(tt.path).RelativeTo(pathx.NewRelPath(tt.base))
			check.AssertSame(h, tt.want, got.String(),
				fmt.Sprintf("RelativeTo(%q, %q)", tt.path, tt.base))
		})
	}

	h.Run("PanicsOnNonAncestor", func(h check.Harness) {
		want := assert.AssertionError{
			Fmt:  "precondition violation: base %q is not an ancestor of %q",
			Args: []any{"b", "a"},
		}
		h.AssertPanicsWith(want, func() {
			_ = pathx.NewRelPath("a").RelativeTo(pathx.NewRelPath("b"))
		})
	})

	h.Run("PanicsOnSiblingPrefix", func(h check.Harness) {
		// "ab" has "a" as a string prefix but is not a path-descendant of "a".
		want := assert.AssertionError{
			Fmt:  "precondition violation: base %q is not an ancestor of %q",
			Args: []any{"a", "ab"},
		}
		h.AssertPanicsWith(want, func() {
			_ = pathx.NewRelPath("ab").RelativeTo(pathx.NewRelPath("a"))
		})
	})
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
		h.Run(tt.path, func(h check.Harness) {
			got := root.LexicallyContains(pathx.NewRelPath(tt.path))
			h.Assertf(got == tt.want, "LexicallyContains(%q) = %v, want %v", tt.path, got, tt.want)
		})
	}
}

func TestAbsPathDir(t *testing.T) {
	h := check.New(t)
	h.Parallel()

	h.Run("Unix", func(h check.Harness) {
		h.Parallel()
		if runtime.GOOS == "windows" {
			h.T().Skip("Unix path semantics")
		}
		tests := []struct {
			path string
			want option.Option[string]
		}{
			{path: "/", want: option.None[string]()},
			{path: "/a", want: option.Some("/")},
			{path: "/a/b", want: option.Some("/a")},
			{path: "/a/b/", want: option.Some("/a/b")},
		}
		for _, tt := range tests {
			h.Run(tt.path, func(h check.Harness) {
				dir, ok := pathx.NewAbsPath(tt.path).Dir().Get()
				got := option.NewOption(dir.String(), ok)
				h.Assertf(option.Compare(tt.want, got) == 0,
					"Dir(%q) = %v, want %v", tt.path, got, tt.want)
			})
		}
	})

	h.Run("Windows", func(h check.Harness) {
		h.Parallel()
		if runtime.GOOS != "windows" {
			h.T().Skip("Windows path semantics")
		}
		tests := []struct {
			path string
			want option.Option[string]
		}{
			{path: `C:\`, want: option.None[string]()},
			{path: `C:\a`, want: option.Some(`C:\`)},
			{path: `C:\a\b`, want: option.Some(`C:\a`)},
			{path: `C:\a\b\`, want: option.Some(`C:\a\b`)},
		}
		for _, tt := range tests {
			h.Run(tt.path, func(h check.Harness) {
				dir, ok := pathx.NewAbsPath(tt.path).Dir().Get()
				got := option.NewOption(dir.String(), ok)
				h.Assertf(option.Compare(tt.want, got) == 0,
					"Dir(%q) = %v, want %v", tt.path, got, tt.want)
			})
		}
	})
}

func TestRelPathDir(t *testing.T) {
	h := check.New(t)
	h.Parallel()

	tests := []struct {
		path string
		want option.Option[string]
	}{
		{path: ".", want: option.None[string]()},
		{path: "a", want: option.None[string]()},
		{path: filepath.Join("a", "b"), want: option.Some("a")},
		{path: filepath.Join("a", "b", "c"), want: option.Some(filepath.Join("a", "b"))},
	}

	for _, tt := range tests {
		h.Run(tt.path, func(h check.Harness) {
			dir, ok := pathx.NewRelPath(tt.path).Dir().Get()
			got := option.NewOption(dir.String(), ok)
			h.Assertf(option.Compare(tt.want, got) == 0,
				"Dir(%q) = %v, want %v", tt.path, got, tt.want)
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

func TestAppendExtension(t *testing.T) {
	h := check.New(t)

	h.Run("AppendsExtension", func(h check.Harness) {
		h.Parallel()

		root := pathx.NewAbsPath(h.T().TempDir())
		path := root.JoinComponents("tool")
		got := path.AppendExtension(".exe")
		check.AssertSame(h, root.JoinComponents("tool.exe").String(), got.String(), "AppendExtension")
	})

	h.Run("PanicsOnEmptyPath", func(h check.Harness) {
		h.Parallel()

		want := assert.AssertionError{Fmt: "precondition violation: %s", Args: []any{"empty path"}}
		h.AssertPanicsWith(want, func() {
			_ = (pathx.AbsPath{}).AppendExtension(".exe")
		})
	})

	h.Run("PanicsOnTrailingSeparator", func(h check.Harness) {
		h.Parallel()

		path := pathx.NewAbsPath(h.T().TempDir() + string(filepath.Separator))
		want := assert.AssertionError{
			Fmt:  "precondition violation: path %q ends with a path separator; so it's not a valid file path",
			Args: []any{path.String()},
		}
		h.AssertPanicsWith(want, func() {
			_ = path.AppendExtension(".exe")
		})
	})
}

func TestLexicallyNormalize(t *testing.T) {
	h := check.New(t)
	sep := string(filepath.Separator)

	h.Run("Examples", func(h check.Harness) {
		tests := []struct {
			path string
			want string
		}{
			{path: "", want: "."},
			{path: "a", want: "a"},
			{path: "a" + sep, want: "a" + sep},
			{path: "a" + sep + sep, want: "a" + sep},
			{path: "." + sep, want: "."},
			{path: ".." + sep, want: ".."},
			{path: "a" + sep + ".." + sep, want: "."},
			{path: "a" + sep + "." + sep + ".." + sep + "b" + sep, want: "b" + sep},
		}
		if runtime.GOOS == "windows" {
			tests = append(tests,
				struct {
					path string
					want string
				}{path: "a/", want: `a\`},
				struct {
					path string
					want string
				}{path: `a\..\c:`, want: `.\c:`},
				struct {
					path string
					want string
				}{path: `\a\..\??\c:\x`, want: `\.\??\c:\x`},
			)
		}

		for _, tt := range tests {
			h.Run(strconv.Quote(tt.path), func(h check.Harness) {
				check.AssertSame(h, tt.want, pathx.LexicallyNormalize(tt.path), "LexicallyNormalize()")
			})
		}
	})

	h.Run("MatchesFilepathCleanWithoutTrailingSeparator", func(h check.Harness) {
		h.Parallel()

		alphabet := []byte{'a', '.', filepath.Separator}
		if runtime.GOOS == "windows" {
			alphabet = append(alphabet, '/')
		}

		forEachPathString(alphabet, 5, func(path string) {
			if hasTrailingTestPathSeparator(path) {
				return
			}
			want := filepath.Clean(path)
			got := pathx.LexicallyNormalize(path)
			h.Assertf(got == want, "LexicallyNormalize(%q) = %q, want %q", path, got, want)
		})
	})
}

func TestRootRelPathBasics(t *testing.T) {
	h := check.New(t)
	root := pathx.NewAbsPath(t.TempDir())
	rel := pathx.NewRelPath(filepath.Join("dir", "file.txt"))
	rootRelPath := pathx.NewRootRelPath(root, rel)
	check.AssertSame(h, rel.String(), rootRelPath.String(), "RootRelPath.String()")
	check.AssertSame(h, root.Join(rel).String(), rootRelPath.AsAbsPath().String(), "RootRelPath.AsAbsPath()")
	check.AssertSame(h, rel.String(), rootRelPath.Rel().String(), "RootRelPath.Rel()")
}

// TestPathFormatting verifies that the path types implement [fmt.Stringer]
// so `%s` / `%q` format them by their String() value, letting callers of
// fmt.Sprintf and errorx.Wrapf pass path types directly without .String().
func TestPathFormatting(t *testing.T) {
	h := check.New(t)

	sep := string(filepath.Separator)
	absPrefix := sep
	if runtime.GOOS == "windows" {
		absPrefix = `C:` + sep
	}
	absStr := absPrefix + filepath.Join("tmp", "x")
	relStr := filepath.Join("a", "b")
	rootRelStr := relStr // RootRelPath.String() returns just the relative portion.

	abs := pathx.NewAbsPath(absStr)
	rel := pathx.NewRelPath(relStr)
	rootRel := pathx.NewRootRelPath(abs, rel)

	tests := []struct {
		name      string
		value     fmt.Stringer
		want      string
		wantQuote string
	}{
		{name: "AbsPath", value: abs, want: absStr, wantQuote: strconv.Quote(absStr)},
		{name: "RelPath", value: rel, want: relStr, wantQuote: strconv.Quote(relStr)},
		{name: "RootRelPath", value: rootRel, want: rootRelStr, wantQuote: strconv.Quote(rootRelStr)},
	}

	baseErr := errorx.Newf("nostack", "base")

	for _, tt := range tests {
		h.Run(tt.name, func(h check.Harness) {
			h.Parallel()

			var sBuilder strings.Builder
			_, err := fmt.Fprintf(&sBuilder, "%s", tt.value)
			h.NoErrorf(err, "fmt.Fprintf %%s")
			gotS := sBuilder.String()
			gotQ := fmt.Sprintf("%q", tt.value)

			gotErrS := errorx.Wrapf("nostack", baseErr, "path: %s", tt.value).Error()
			wantErrS := "path: " + tt.want + ": base"

			gotErrQ := errorx.Wrapf("nostack", baseErr, "path: %q", tt.value).Error()
			wantErrQ := "path: " + tt.wantQuote + ": base"

			check.AssertSame(h, tt.want, gotS, "fmt.Sprintf %s")
			check.AssertSame(h, tt.wantQuote, gotQ, "fmt.Sprintf %q")
			check.AssertSame(h, wantErrS, gotErrS, "errorx.Wrapf %s")
			check.AssertSame(h, wantErrQ, gotErrQ, "errorx.Wrapf %q")
		})
	}
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
	}

	for _, tt := range tests {
		h.Run(tt.name, func(h check.Harness) {
			h.AssertPanicsWith(want, tt.call)
		})
	}
}

func TestHasPathSeparators(t *testing.T) {
	h := check.New(t)

	tests := []struct {
		name  string
		value string
		want  bool
	}{
		{name: "empty", value: "", want: false},
		{name: "plain", value: "abc", want: false},
		{name: "slash", value: "a/b", want: true},
	}
	if runtime.GOOS == "windows" {
		tests = append(tests, struct {
			name  string
			value string
			want  bool
		}{name: "backslash", value: `a\b`, want: true})
	}

	for _, tt := range tests {
		h.Run(tt.name, func(h check.Harness) {
			got := pathx.HasPathSeparators(tt.value)
			h.Assertf(got == tt.want, "HasPathSeparators(%q) = %v, want %v", tt.value, got, tt.want)
		})
	}
}

func forEachPathString(alphabet []byte, maxLen int, yield func(string)) {
	buf := make([]byte, maxLen)
	var visit func(n, pos int)
	visit = func(n, pos int) {
		if pos == n {
			yield(string(buf[:n]))
			return
		}
		for _, c := range alphabet {
			buf[pos] = c
			visit(n, pos+1)
		}
	}
	for n := 0; n <= maxLen; n++ {
		visit(n, 0)
	}
}

func hasTrailingTestPathSeparator(path string) bool {
	if path == "" {
		return false
	}
	last := path[len(path)-1]
	if runtime.GOOS == "windows" {
		return last == '\\' || last == '/'
	}
	return last == filepath.Separator
}
