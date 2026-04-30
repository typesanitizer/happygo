package fsx_walk_test

import (
	"iter"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/typesanitizer/happygo/common/check"
	. "github.com/typesanitizer/happygo/common/check/prelude"
	. "github.com/typesanitizer/happygo/common/core"
	"github.com/typesanitizer/happygo/common/core/pathx"
	"github.com/typesanitizer/happygo/common/fsx/fsx_testkit"
	"github.com/typesanitizer/happygo/common/fsx/fsx_walk"
	"github.com/typesanitizer/happygo/common/iterx"
	"github.com/typesanitizer/happygo/common/source_code"
	"github.com/typesanitizer/happygo/common/syscaps"
)

type WalkResultSeq = iter.Seq[Result[fsx_walk.FSWalkEntry]]

func TestWalk(t *testing.T) {
	h := check.New(t)
	h.Parallel()

	h.Run("GitIgnore", testGitIgnore)
	h.Run("Faults", testFaults)

	h.Run("Traversal", func(h check.Harness) {
		h.Parallel()

		h.Run("SkipSubtree", func(h check.Harness) {
			h.Parallel()

			fs := fsx_testkit.NewMemFS(h)
			fsx_testkit.WriteTree(h, fs, map[string]string{
				"a/":           "",
				"a/skip/":      "",
				"a/skip/y.txt": "y",
				"a/x.txt":      "x",
				"b.txt":        "b",
			})

			entries := Do(fsx_walk.WalkNonDet(fs, pathx.Dot(), fsx_walk.WalkOptions{RespectGitIgnore: false}))(h)
			got := collectPaths(h, entries, collectOptions{skipDir: func(name string) bool {
				return name == "a/skip"
			}})
			checkVisitedPaths(h, []string{"a", "a/skip", "a/x.txt", "b.txt"}, got)
		})
	})
	h.Run("Root", func(h check.Harness) {
		h.Parallel()

		h.Run("SymlinkNotFollowed", func(h check.Harness) {
			h.Parallel()

			parentFS := fsx_testkit.TempDirFS(h)
			parent := parentFS.Root().String()
			fsx_testkit.WriteTree(h, parentFS, map[string]string{"target/file.txt": "x"})

			target := filepath.Join(parent, "target")
			link := filepath.Join(parent, "link")
			support := Do(fsx_testkit.TrySymlink(target, link))(h)
			if support.IsUnsupported() {
				h.T().Skipf("symlinks not supported on this platform")
			}

			fs := Do(syscaps.FS(pathx.NewAbsPath(link)))(h)

			_, err := fsx_walk.WalkNonDet(fs, pathx.Dot(), fsx_walk.WalkOptions{RespectGitIgnore: false})
			walkErr := requireWalkError(h, err, fsx_walk.WalkErrorKind_RootNotDir)
			h.Assertf(strings.Contains(walkErr.Error(), "not a directory"),
				"WalkError.Error() = %q, want not-a-directory message", walkErr.Error())
		})
	})
}

func testGitIgnore(h check.Harness) {
	h.Parallel()

	h.Run("RespectGitIgnore", func(h check.Harness) {
		h.Parallel()

		fs := fsx_testkit.NewMemFS(h)
		fsx_testkit.WriteTree(h, fs, map[string]string{
			".git": "gitdir: root\n",
			".gitignore": `.git
ignored/
*.log
`,
			"ignored/":         "",
			"ignored/file.txt": "ignored",
			"sub/":             "",
			"sub/.gitignore":   "!keep.log\n",
			"sub/drop.log":     "drop",
			"sub/file.txt":     "file",
			"sub/keep.log":     "keep",
		})

		entries := Do(fsx_walk.WalkNonDet(fs, pathx.Dot(), fsx_walk.WalkOptions{RespectGitIgnore: true}))(h)
		got := collectPaths(h, entries)
		checkVisitedPaths(h,
			[]string{".gitignore", "sub", "sub/.gitignore", "sub/file.txt", "sub/keep.log"},
			got,
		)
	})

	h.Run("RequiresFSRootRepo", func(h check.Harness) {
		h.Parallel()

		fs := fsx_testkit.NewMemFS(h)
		fsx_testkit.WriteTree(h, fs, map[string]string{
			".gitignore": "*.txt\n",
			"file.txt":   "x",
		})

		_, err := fsx_walk.WalkNonDet(fs, pathx.Dot(), fsx_walk.WalkOptions{RespectGitIgnore: true})
		walkErr := requireWalkError(h, err, fsx_walk.WalkErrorKind_FSRootNotRepo)
		check.AssertSame(h, fs.Root(), walkErr.Path(), "WalkError.Path()")
		h.Assertf(strings.Contains(walkErr.Error(), "not a git repository root"),
			"WalkError.Error() = %q, want repository-root message", walkErr.Error())
		h.NoErrorf(walkErr.Unwrap(), "WalkError.Unwrap()")
	})

	h.Run("ExplicitSubtree", testGitIgnore_OnExplicitSubtree)
	h.Run("NestedRepoReset", testGitIgnore_ResetsAtNestedRepo)
	h.Run("IgnoredNestedRepo", testGitIgnore_SkipsIgnoredNestedRepo)
	h.Run("ParseErrors", testGitIgnore_PreservesAllParseErrors)
	h.Run("ConcurrentSiblingWarnings", testGitIgnore_ConcurrentSiblingWarnings)
}

func testFaults(h check.Harness) {
	h.Parallel()

	h.Run("InitialRootStat", func(h check.Harness) {
		h.Parallel()

		fs := fsx_testkit.NewMemFS(h)
		fsx_testkit.WriteTree(h, fs, map[string]string{"a.txt": "a"})
		h.NoErrorf(fs.RemoveAll(pathx.Dot()), "RemoveAll(.)")

		_, err := fsx_walk.WalkNonDet(fs, pathx.Dot(), fsx_walk.WalkOptions{RespectGitIgnore: false})
		walkErr := requireWalkError(h, err, fsx_walk.WalkErrorKind_IOFailed)
		check.AssertSame(h, fs.Root(), walkErr.Path(), "WalkError.Path()")
		h.Assertf(walkErr.Unwrap() != nil, "WalkError.Unwrap() = nil, want underlying error")
		h.Assertf(strings.Contains(walkErr.Error(), fs.Root().String()),
			"got: WalkError.Error() = %q\nwant: root path %v to appear in it", walkErr.Error(), fs.Root())
	})

	h.Run("InitialGitStat", func(h check.Harness) {
		h.Parallel()

		fs := fsx_testkit.NewFaultyFS(h, fsx_testkit.NewMemFS(h), fsx_testkit.Fault{Op: fsx_testkit.FaultOp_Stat, Rel: pathx.NewRelPath(".git")})

		_, err := fsx_walk.WalkNonDet(fs, pathx.Dot(), fsx_walk.WalkOptions{RespectGitIgnore: true})
		_ = requireWalkError(h, err, fsx_walk.WalkErrorKind_IOFailed)
	})

	h.Run("IterateGitStat", func(h check.Harness) {
		h.Parallel()

		baseFS := fsx_testkit.NewMemFS(h)
		fsx_testkit.WriteTree(h, baseFS, map[string]string{
			".git": "gitdir: root\n",
			"a/":   "",
		})
		fs := fsx_testkit.NewFaultyFS(h, baseFS, fsx_testkit.Fault{Op: fsx_testkit.FaultOp_Stat, Rel: pathx.NewRelPath("a/.git")})

		entries := Do(fsx_walk.WalkNonDet(fs, pathx.Dot(), fsx_walk.WalkOptions{RespectGitIgnore: true}))(h)
		_ = requireWalkError(h, firstWalkError(h, entries), fsx_walk.WalkErrorKind_IOFailed)
	})

	h.Run("IterateGitIgnoreRead", func(h check.Harness) {
		h.Parallel()

		baseFS := fsx_testkit.NewMemFS(h)
		fsx_testkit.WriteTree(h, baseFS, map[string]string{
			".git": "gitdir: root\n",
			"a/":   "",
		})
		fs := fsx_testkit.NewFaultyFS(h, baseFS, fsx_testkit.Fault{Op: fsx_testkit.FaultOp_Open, Rel: pathx.NewRelPath("a/.gitignore")})

		entries := Do(fsx_walk.WalkNonDet(fs, pathx.Dot(), fsx_walk.WalkOptions{RespectGitIgnore: true}))(h)
		_ = requireWalkError(h, firstWalkError(h, entries), fsx_walk.WalkErrorKind_IOFailed)
	})

	h.Run("IterateReadDir", func(h check.Harness) {
		h.Parallel()

		baseFS := fsx_testkit.NewMemFS(h)
		fsx_testkit.WriteTree(h, baseFS, map[string]string{
			"a/": "",
		})
		fs := fsx_testkit.NewFaultyFS(h, baseFS, fsx_testkit.Fault{Op: fsx_testkit.FaultOp_Open, Rel: pathx.NewRelPath("a")})

		entries := Do(fsx_walk.WalkNonDet(fs, pathx.Dot(), fsx_walk.WalkOptions{RespectGitIgnore: false}))(h)
		_ = requireWalkError(h, firstWalkError(h, entries), fsx_walk.WalkErrorKind_IOFailed)
	})
}

func testGitIgnore_OnExplicitSubtree(h check.Harness) {
	fs := fsx_testkit.NewMemFS(h)
	fsx_testkit.WriteTree(h, fs, map[string]string{
		".git": "gitdir: root\n",
		".gitignore": `.git
blocked/
*.tmp
`,
		"blocked/":                "",
		"blocked/.gitignore":      "!child/\n",
		"blocked/child/":          "",
		"blocked/child/file.txt":  "blocked",
		"open/":                   "",
		"open/.gitignore":         "drop/\n",
		"open/child/":             "",
		"open/child/drop/":        "",
		"open/child/drop/file.go": "drop",
		"open/child/file.go":      "open",
		"open/child/root.tmp":     "ignored",
	})

	h.Run("Ignores root subtree when blocked by ancestor", func(h check.Harness) {
		h.Parallel()
		_, err := fsx_walk.WalkNonDet(fs, pathx.NewRelPath("blocked/child"), fsx_walk.WalkOptions{RespectGitIgnore: true})
		walkErr := requireWalkError(h, err, fsx_walk.WalkErrorKind_RootIsIgnored)
		check.AssertSame(h,
			source_code.Snippet{
				Path:     pathx.NewRelPath(".gitignore"),
				Position: source_code.NewPosition(2, 1),
				Text:     "blocked/",
			},
			walkErr.GitIgnorePattern(),
			"GitIgnorePattern()",
			cmp.AllowUnexported(source_code.Position{}),
		)
		h.Assertf(strings.Contains(walkErr.Error(), "blocked/"),
			"WalkError.Error() = %q, want ignored pattern", walkErr.Error())
	})

	h.Run("Walks subtree with ancestor gitignore matchers", func(h check.Harness) {
		h.Parallel()
		entries := Do(fsx_walk.WalkNonDet(fs, pathx.NewRelPath("open/child"), fsx_walk.WalkOptions{RespectGitIgnore: true}))(h)
		got := collectPaths(h, entries)
		checkVisitedPaths(h, []string{"file.go"}, got)
	})

	h.Run("Walks subtree when gitignore is disabled", func(h check.Harness) {
		h.Parallel()
		entries := Do(fsx_walk.WalkNonDet(fs, pathx.NewRelPath("blocked/child"), fsx_walk.WalkOptions{RespectGitIgnore: false}))(h)
		got := collectPaths(h, entries)
		checkVisitedPaths(h, []string{"file.txt"}, got)
	})
}

func testGitIgnore_ResetsAtNestedRepo(h check.Harness) {
	fs := fsx_testkit.NewMemFS(h)
	fsx_testkit.WriteTree(h, fs, map[string]string{
		".git": "gitdir: root\n",
		".gitignore": `.git
*.txt
`,
		"root.txt":              "root",
		"nested/":               "",
		"nested/.git":           "gitdir: nested\n",
		"nested/child/":         "",
		"nested/child/file.txt": "inside",
		"nested/inside.txt":     "inside",
	})

	h.Run("WhileWalking", func(h check.Harness) {
		h.Parallel()
		entries := Do(fsx_walk.WalkNonDet(fs, pathx.Dot(), fsx_walk.WalkOptions{RespectGitIgnore: true}))(h)
		got := collectPaths(h, entries)
		checkVisitedPaths(
			h,
			[]string{
				".gitignore", "nested", "nested/.git", "nested/child",
				"nested/child/file.txt", "nested/inside.txt",
			},
			got,
		)
	})

	h.Run("ExplicitSubtree", func(h check.Harness) {
		h.Parallel()
		entries := Do(fsx_walk.WalkNonDet(fs, pathx.NewRelPath("nested/child"), fsx_walk.WalkOptions{RespectGitIgnore: true}))(h)
		got := collectPaths(h, entries)
		checkVisitedPaths(h, []string{"file.txt"}, got)
	})
}

func testGitIgnore_SkipsIgnoredNestedRepo(h check.Harness) {
	h.Parallel()

	fs := fsx_testkit.NewMemFS(h)
	fsx_testkit.WriteTree(h, fs, map[string]string{
		".git": "gitdir: root\n",
		".gitignore": `.git
nested/
`,
		"root.txt":          "root",
		"nested/":           "",
		"nested/.git":       "gitdir: nested\n",
		"nested/inside.txt": "inside",
	})

	entries := Do(fsx_walk.WalkNonDet(fs, pathx.Dot(), fsx_walk.WalkOptions{RespectGitIgnore: true}))(h)
	got := collectPaths(h, entries)
	checkVisitedPaths(h, []string{".gitignore", "root.txt"}, got)
}

func testGitIgnore_PreservesAllParseErrors(h check.Harness) {
	h.Parallel()

	fs := fsx_testkit.NewMemFS(h)
	fsx_testkit.WriteTree(h, fs, map[string]string{
		".git": "gitdir: root\n",
		".gitignore": `!
** *
`,
		"a.txt": "a",
	})

	entries := Do(fsx_walk.WalkNonDet(fs, pathx.Dot(), fsx_walk.WalkOptions{RespectGitIgnore: true}))(h)
	err := firstWalkError(h, entries)
	walkErr := requireWalkError(h, err, fsx_walk.WalkErrorKind_ParseGitIgnore)
	h.Assertf(strings.Count(walkErr.Error(), "invalid pattern") == 2,
		"WalkError.Error() = %q, want 2 invalid pattern errors", walkErr.Error())
	parseErrs, ok := walkErr.Unwrap().(interface{ Unwrap() []error })
	h.Assertf(ok, "WalkError.Unwrap() = %T, want an error with Unwrap() []error", walkErr.Unwrap())
	h.Assertf(len(parseErrs.Unwrap()) == 2, "parse error count = %d, want 2", len(parseErrs.Unwrap()))

	parseSnippets, ok := walkErr.Unwrap().(interface{ Snippets() []source_code.Snippet })
	h.Assertf(ok, "WalkError.Unwrap() = %T, want an error with Snippets() []source_code.Snippet", walkErr.Unwrap())
	check.AssertSame(h,
		[]source_code.Snippet{
			{Path: pathx.NewRelPath(".gitignore"), Position: source_code.NewPosition(1, 2), Text: "!"},
			{Path: pathx.NewRelPath(".gitignore"), Position: source_code.NewPosition(2, 4), Text: "** *"},
		},
		parseSnippets.Snippets(),
		"parse error snippets",
		cmp.AllowUnexported(source_code.Position{}),
	)
}

func testGitIgnore_ConcurrentSiblingWarnings(h check.Harness) {
	h.Parallel()

	fs := fsx_testkit.NewMemFS(h)
	fsx_testkit.WriteTree(h, fs, map[string]string{
		".git":         "gitdir: root\n",
		"a/":           "",
		"a/.gitignore": "!\n",
		"a/file.txt":   "a",
		"b/":           "",
		"b/.gitignore": "** *\n",
		"b/file.txt":   "b",
	})

	entries := Do(fsx_walk.WalkNonDet(fs, pathx.Dot(), fsx_walk.WalkOptions{RespectGitIgnore: true}))(h)
	children := make(map[string]WalkResultSeq)
	for entryRes := range entries {
		entry := Do(entryRes.Get())(h)
		name := entry.Name().String()
		if entry.IsDir() && (name == "a" || name == "b") {
			children[name] = entry.ChildrenNonDet()
		}
	}
	h.Assertf(len(children) == 2, "children = %v, want a and b", children)

	type childResult struct {
		name string
		errs []error
	}
	start := make(chan struct{})
	results := make(chan childResult, len(children))
	var wg sync.WaitGroup
	for name, seq := range children {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			var errs []error
			for entryRes := range seq {
				_, err := entryRes.Get()
				if err != nil {
					errs = append(errs, err)
				}
			}
			results <- childResult{name: name, errs: errs}
		}()
	}
	close(start)
	wg.Wait()
	close(results)

	got := make(map[string][]string)
	for res := range results {
		for _, err := range res.errs {
			walkErr := requireWalkError(h, err, fsx_walk.WalkErrorKind_ParseGitIgnore)
			got[res.name] = append(got[res.name], walkErr.Path().String())
		}
	}
	check.AssertSame(h,
		map[string][]string{
			"a": {fs.Root().Join(pathx.NewRelPath("a/.gitignore")).String()},
			"b": {fs.Root().Join(pathx.NewRelPath("b/.gitignore")).String()},
		},
		got,
		"parse warning paths",
		cmpopts.SortSlices(strings.Compare),
	)
}

func checkVisitedPaths(h check.Harness, want []string, got []string) {
	h.T().Helper()
	check.AssertSame(h, want, got, "visited paths", cmpopts.SortSlices(strings.Compare))
}

type collectOptions struct {
	skipDir func(path string) bool
}

// collectPaths recursively collects all paths from the walk tree.
func collectPaths(h check.Harness, entries WalkResultSeq, opts ...collectOptions) []string {
	h.T().Helper()
	var opt collectOptions
	for _, v := range opts {
		if v.skipDir != nil {
			opt.skipDir = v.skipDir
		}
	}
	var impl func(WalkResultSeq, string) []string
	impl = func(entries WalkResultSeq, prefix string) []string {
		var paths []string
		for entryRes := range entries {
			entry := Do(entryRes.Get())(h)
			path := joinWalkPath(prefix, entry.Name().String())
			paths = append(paths, path)
			if entry.IsDir() {
				if opt.skipDir != nil && opt.skipDir(path) {
					continue
				}
				paths = append(paths, impl(entry.ChildrenNonDet(), path)...)
			}
		}
		return paths
	}
	return impl(entries, "")
}

func joinWalkPath(prefix, name string) string {
	if prefix == "" {
		return name
	}
	return prefix + "/" + name
}

func firstWalkError(h check.Harness, entries WalkResultSeq) error {
	h.T().Helper()
	var impl func(WalkResultSeq) Option[error]
	impl = func(entries WalkResultSeq) Option[error] {
		return iterx.Find(entries, func(entryRes Result[fsx_walk.FSWalkEntry]) Option[error] {
			entry, err := entryRes.Get()
			if err != nil {
				return Some(err)
			}
			if entry.IsDir() {
				return impl(entry.ChildrenNonDet())
			}
			return None[error]()
		})
	}
	err, ok := impl(entries).Get()
	h.Assertf(ok, "walk completed without an error")
	return err
}

func requireWalkError(h check.Harness, err error, wantKind fsx_walk.WalkErrorKind) *fsx_walk.WalkError {
	h.T().Helper()
	h.Assertf(err != nil, "WalkNonDet unexpectedly succeeded")
	walkErr, ok := err.(*fsx_walk.WalkError) // direct cast for result of Walk is OK
	h.Assertf(ok, "WalkNonDet error type = %T, want *WalkError", err)
	h.Assertf(walkErr.Kind() == wantKind,
		"WalkNonDet error kind = %v, want %v", walkErr.Kind(), wantKind)
	return walkErr
}
