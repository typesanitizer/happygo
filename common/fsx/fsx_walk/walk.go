// Copyright 2026 Varun Gandhi
//
// SPDX-License-Identifier: PolyForm-Noncommercial-1.0.0

// Package fsx_walk provides directory walking over fsx.FS with optional
// .gitignore handling.
package fsx_walk

import (
	"bytes"
	"iter"
	"slices"

	"github.com/boyter/gocodewalker/go-gitignore"

	"github.com/typesanitizer/happygo/common/assert"
	"github.com/typesanitizer/happygo/common/core/option"
	"github.com/typesanitizer/happygo/common/core/pathx"
	"github.com/typesanitizer/happygo/common/core/result"
	"github.com/typesanitizer/happygo/common/errorx"
	"github.com/typesanitizer/happygo/common/fsx"
	"github.com/typesanitizer/happygo/common/iterx"
	"github.com/typesanitizer/happygo/common/source_code"
)

// WalkNonDet returns an iterator over the entries in the [root] directory.
//
// The iterator yields entries in non-deterministic order.
// Directory entries carry a lazy [FSWalkEntry.ChildrenNonDet] iterator;
// descending into a directory is the caller's choice.
// Stopping early is done by breaking out of the iteration.
//
// Initial errors — returned via the error return (no iterator is created):
//   - [WalkErrorKind_IOFailed]: root cannot be stat'd or an ancestor .gitignore
//     cannot be read.
//   - [WalkErrorKind_RootNotDir]: root exists but is not a directory (e.g. a
//     symlink or regular file).
//   - [WalkErrorKind_FSRootNotRepo]: [WalkOptions.RespectGitIgnore] is set but
//     the filesystem root is not a git repository root.
//   - [WalkErrorKind_RootIsIgnored]: [WalkOptions.RespectGitIgnore] is set and
//     root is excluded by an ancestor .gitignore rule.
//
// Inside the Result for an iterator, there are two kinds of errors.
//
// - Terminal errors, after which no further elements are yielded for that directory.
//   - [WalkErrorKind_IOFailed]: a directory could not be read, or some other stat
//     operation (e.g. for .git/.gitignore) failed with an error other than
//     "does not exist".
//
// - Non-terminal errors, after which further elements may be yielded:
//   - [WalkErrorKind_ParseGitIgnore]: a .gitignore file could not be parsed.
//     The walk continues without that file's rules.
//     If this error is returned, WalkError.Unwrap() returns a multi-error which
//     carries all parsing errors via Unwrap() []error.
func WalkNonDet(fs fsx.FS, root pathx.RelPath, opts WalkOptions) (iter.Seq[result.Result[FSWalkEntry]], error) {
	info, err := fs.Stat(root, fsx.StatOptions{FollowFinalSymlink: false, OnErrorTraverseParents: true})
	if err != nil {
		return nil, newWalkError(WalkErrorKind_IOFailed, fs.Root().Join(root)).withErr(err)
	}
	if !info.IsDir() {
		return nil, newWalkError(WalkErrorKind_RootNotDir, fs.Root().Join(root))
	}

	w := &walker{fs: fs, opts: opts}
	if opts.RespectGitIgnore {
		repoRoot, err := w.isGitRepoRoot(pathx.Dot())
		if err != nil {
			return nil, err
		}
		if !repoRoot {
			return nil, newWalkError(WalkErrorKind_FSRootNotRepo, fs.Root())
		}
	}

	var matchers []gitIgnoreMatcher
	var warnings []*WalkError
	if opts.RespectGitIgnore && root.String() != "." {
		matchers, warnings, err = w.ancestorGitIgnores(root)
		if err != nil {
			return nil, err
		}
	}
	warningSeq := iterx.Map(iterx.FromSlice(warnings), fail)
	return iterx.Chain(warningSeq, w.walkDir(root, matchers)), nil
}

// NOTE(id: fsx-walk-concurrent-children): fsx_walk must not share mutable
// traversal state across child iterators for distinct directories. Keep walker
// immutable; per-directory state such as parse warnings must flow through
// return values.
type walker struct {
	fs   fsx.FS
	opts WalkOptions
}

func (w *walker) walkDir(dir pathx.RelPath, parentMatchers []gitIgnoreMatcher) iter.Seq[result.Result[FSWalkEntry]] {
	return walkDirSeq{w: w, dir: dir, parentMatchers: parentMatchers}.iterate
}

type gitIgnoreMatcher struct {
	matcher gitignore.GitIgnore
	// dir is the directory containing the .gitignore, relative to the fs root.
	dir        pathx.RelPath
	ignoreFile pathx.RelPath
}

type walkDirSeq struct {
	w              *walker
	dir            pathx.RelPath
	parentMatchers []gitIgnoreMatcher
}

// iterate yields the entries of seq.dir.
//
// Terminal [Failure] errors (no further elements yielded):
//   - [WalkErrorKind_IOFailed]: the .git stat for the repo-root check failed,
//     a .gitignore could not be read, or [fsx.FS.ReadDir] returned an error.
//
// Non-terminal [Failure] errors (iteration continues):
//   - [WalkErrorKind_ParseGitIgnore]: the directory's .gitignore could not be
//     parsed. The walk continues without that file's rules.
func (seq walkDirSeq) iterate(yield func(result.Result[FSWalkEntry]) bool) {
	activeMatchers := seq.parentMatchers
	if seq.w.opts.RespectGitIgnore {
		// If this directory is a nested repo root, discard inherited matchers
		// so the new repo starts with a clean ignore state.
		repoRoot, walkErr := seq.w.isGitRepoRoot(seq.dir)
		if walkErr != nil {
			yield(fail(walkErr))
			return
		}
		if repoRoot {
			activeMatchers = nil
		}
		var warnings []*WalkError
		activeMatchers, warnings, walkErr = seq.w.childGitIgnores(seq.dir, activeMatchers)
		if walkErr != nil {
			yield(fail(walkErr))
			return
		}
		for _, warning := range warnings {
			if !yield(fail(warning)) {
				return
			}
		}
	}
	// Clip so that children sharing this slice can't corrupt each other
	// via spare capacity when childGitIgnores appends.
	activeMatchers = slices.Clip(activeMatchers)

	for entryRes := range seq.w.fs.ReadDir(seq.dir) {
		de, err := entryRes.Get()
		if err != nil {
			yield(fail(newWalkError(WalkErrorKind_IOFailed, seq.w.fs.Root().Join(seq.dir)).withErr(err)))
			return
		}

		name := de.BaseName()
		if seq.w.opts.RespectGitIgnore && name.String() == ".git" {
			continue
		}
		child := seq.dir.JoinComponents(name.String())
		isDir := de.IsDir()

		if seq.w.opts.RespectGitIgnore {
			_, ignored := seq.w.identifyPatternIgnoring(activeMatchers, child, isDir).Get()
			if ignored {
				continue
			}
		}

		var entry FSWalkEntry
		if isDir {
			entry = FSWalkEntry{name: name, children: seq.w.walkDir(child, activeMatchers)}
		} else {
			entry = FSWalkEntry{name: name, children: nil}
		}

		if !yield(result.Success(entry)) {
			return
		}
	}
}

// WalkOptions configures [WalkNonDet].
type WalkOptions struct {
	// RespectGitIgnore enables .gitignore handling while walking.
	//
	// NOTE: This does not cover the following situations:
	// - .git/info/exclude files
	// - global configuration via core.excludesFile
	// - Handling for the GIT_DIR environment variable
	RespectGitIgnore bool
}

// FSWalkEntry is a single entry yielded by [WalkNonDet].
//
// It is either a file (carrying just its name) or a directory
// (carrying its name and a lazily-evaluated iterator of children).
// Use [FSWalkEntry.IsDir] to distinguish the two cases.
type FSWalkEntry struct {
	name     fsx.Name
	children iter.Seq[result.Result[FSWalkEntry]] // nil for files
}

// Name returns the basename of this entry.
func (e FSWalkEntry) Name() fsx.Name {
	return e.name
}

// IsDir reports whether this entry is a directory.
func (e FSWalkEntry) IsDir() bool {
	return e.children != nil
}

// ChildrenNonDet returns an iterator over the directory's children.
//
// Pre-condition: e.IsDir() is true.
func (e FSWalkEntry) ChildrenNonDet() iter.Seq[result.Result[FSWalkEntry]] {
	assert.Preconditionf(e.IsDir(), "ChildrenNonDet() called on file entry %q", e.name)
	return e.children
}

// ancestorGitIgnores returns the .gitignore matchers inherited from ancestors
// of root. It does not load root's own .gitignore — that is loaded lazily by
// [walkDirSeq.iterate].
//
// Pre-conditions:
//   - [WalkOptions.RespectGitIgnore] is enabled.
//   - root is not ".".
//   - (Unchecked) the filesystem root has been verified as a git repo root.
//
// Errors:
//   - [WalkErrorKind_RootIsIgnored]: an ancestor .gitignore rule excludes root
//     (or excludes root itself relative to its enclosing matchers).
//   - [WalkErrorKind_IOFailed]: a .git stat or .gitignore read failed along the
//     ancestor path.
func (w *walker) ancestorGitIgnores(root pathx.RelPath) ([]gitIgnoreMatcher, []*WalkError, error) {
	assert.Preconditionf(w.opts.RespectGitIgnore, "ancestorGitIgnores called with RespectGitIgnore=false")
	assert.Preconditionf(root != pathx.Dot(), "ancestorGitIgnores called with root=.")

	matchers, warnings, err := w.childGitIgnores(pathx.Dot(), nil)
	if err != nil {
		return nil, nil, err
	}

	for ancestor := range root.Ancestors() {
		if pattern, ignored := w.identifyPatternIgnoring(matchers, ancestor, true).Get(); ignored {
			return nil, nil, newWalkError(WalkErrorKind_RootIsIgnored, w.fs.Root().Join(root)).withGitIgnorePattern(pattern)
		}
		repoRoot, err := w.isGitRepoRoot(ancestor)
		if err != nil {
			return nil, nil, err
		}
		if repoRoot {
			matchers = nil
		}
		var childWarnings []*WalkError
		matchers, childWarnings, err = w.childGitIgnores(ancestor, matchers)
		if err != nil {
			return nil, nil, err
		}
		warnings = append(warnings, childWarnings...)
	}

	if pattern, ignored := w.identifyPatternIgnoring(matchers, root, true).Get(); ignored {
		return nil, nil, newWalkError(WalkErrorKind_RootIsIgnored, w.fs.Root().Join(root)).withGitIgnorePattern(pattern)
	}
	return matchers, warnings, nil
}

// childGitIgnores returns the matcher set that applies while visiting dir's
// children.
//
// If dir has a .gitignore file, its matcher is appended after parent so that
// deeper directories see the usual nested precedence.
//
// Pre-condition: [WalkOptions.RespectGitIgnore] is enabled.
func (w *walker) childGitIgnores(dir pathx.RelPath, parent []gitIgnoreMatcher) ([]gitIgnoreMatcher, []*WalkError, *WalkError) {
	assert.Preconditionf(w.opts.RespectGitIgnore, "childGitIgnores called with RespectGitIgnore=false")
	matcher, warnings, walkErr := w.loadGitIgnore(dir)
	if walkErr != nil {
		return nil, nil, walkErr
	}
	if matcher == nil {
		return parent, warnings, nil
	}
	return append(parent, *matcher), warnings, nil
}

func (w *walker) isGitRepoRoot(dir pathx.RelPath) (bool, *WalkError) {
	gitRel := dir.JoinComponents(".git")
	info, err := w.fs.Stat(gitRel, fsx.StatOptions{FollowFinalSymlink: false, OnErrorTraverseParents: false})
	if err != nil {
		if errorx.GetRootCauseAsValue(err, fsx.ErrNotExist) {
			return false, nil
		}
		return false, newWalkError(WalkErrorKind_IOFailed, w.fs.Root().Join(gitRel)).withErr(err)
	}
	mode := info.Mode()
	return mode.IsDir() || mode.IsRegular(), nil
}

func (w *walker) loadGitIgnore(dir pathx.RelPath) (*gitIgnoreMatcher, []*WalkError, *WalkError) {
	ignoreRel := dir.JoinComponents(".gitignore")
	content, err := w.fs.ReadFile(ignoreRel)
	if err != nil {
		if errorx.GetRootCauseAsValue(err, fsx.ErrNotExist) {
			return nil, nil, nil
		}
		return nil, nil, newWalkError(WalkErrorKind_IOFailed, w.fs.Root().Join(ignoreRel)).withErr(err)
	}

	var parseErrs []gitignore.Error
	// Oddity: gitignore.New expects that the second argument is the _absolute path_
	// to the directory containing the .gitignore file, so that it can correctly
	// interpret repo-root-relative paths in the .gitignore.
	//
	// I don't fully understand why this makes sense, I don't think it does,
	// but leaving this for now to avoid having to fork yet more code/
	// hand-roll this ourselves...
	matcher := gitignore.New(bytes.NewReader(content), w.fs.Root().Join(dir).String(), func(err gitignore.Error) bool {
		parseErrs = append(parseErrs, err)
		return true
	})
	if len(parseErrs) > 0 {
		warning := newWalkError(WalkErrorKind_ParseGitIgnore, w.fs.Root().Join(ignoreRel)).withErr(newGitIgnoreParseErrors(ignoreRel, content, parseErrs))
		return nil, []*WalkError{warning}, nil
	}
	return &gitIgnoreMatcher{matcher: matcher, dir: dir, ignoreFile: ignoreRel}, nil, nil
}

// identifyPatternIgnoring looks through the matchers with last-wins
// semantics, and checks if rel should be ignored overall.
//
// If rel should be ignored, then the most appropriate pattern
// from the matchers is returned. Otherwise, None is returned.
//
// isDir denotes whether rel corresponds to a directory path.
func (w *walker) identifyPatternIgnoring(matchers []gitIgnoreMatcher, rel pathx.RelPath, isDir bool) option.Option[source_code.Snippet] {
	ignored := option.None[source_code.Snippet]()
	// Always walk all matches from start-to-end, because later
	// entries correspond to nearer .gitignore files, which override
	// patterns in more distant .gitignore files.
	for _, m := range matchers {
		// gitignore.GitIgnore.Relative expects a path relative to the directory
		// containing the .gitignore (i.e. m.dir), not relative to the fs root.
		match := m.matcher.Relative(rel.RelativeTo(m.dir).String(), isDir)
		if match == nil {
			continue
		}
		if match.Ignore() {
			ignored = option.Some(sourceCodeSnippet(m.ignoreFile, match))
		} else { // !ignore => keep, so reset 'ignored'.
			ignored = option.None[source_code.Snippet]()
		}
	}
	return ignored
}

func fail(e *WalkError) result.Result[FSWalkEntry] {
	return result.Failure[FSWalkEntry](e)
}

func sourceCodeSnippet(ignoreFile pathx.RelPath, match gitignore.Match) source_code.Snippet {
	position := match.Position()
	return source_code.Snippet{
		Path:     ignoreFile,
		Position: source_code.NewPosition(int32(position.Line), int32(position.Column)),
		Text:     match.String(),
	}
}
