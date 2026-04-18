package fsx_walk

import (
	"fmt"
	"slices"
	"strings"

	"github.com/boyter/gocodewalker/go-gitignore"

	"github.com/typesanitizer/happygo/common/assert"
	"github.com/typesanitizer/happygo/common/core/option"
	"github.com/typesanitizer/happygo/common/core/pathx"
	"github.com/typesanitizer/happygo/common/source_code"
)

// WalkErrorKind classifies a [WalkError].
type WalkErrorKind uint8

const (
	// WalkErrorKind_IOFailed indicates a filesystem I/O failure (stat, read
	// directory, read file).
	//
	// Methods returning data:
	//   - [WalkError.Path]: absolute path of the failing operation.
	//   - [WalkError.Unwrap]: the underlying I/O error.
	WalkErrorKind_IOFailed WalkErrorKind = iota + 1
	// WalkErrorKind_RootNotDir indicates the walk root exists but is not a
	// directory.
	//
	// Methods returning data:
	//   - [WalkError.Path]: absolute path of the walk root.
	WalkErrorKind_RootNotDir
	// WalkErrorKind_ParseGitIgnore indicates a .gitignore file could not be
	// parsed. This is non-fatal: the walk continues without that file's rules.
	//
	// Methods returning data:
	//   - [WalkError.Path]: absolute path of the .gitignore file.
	//   - [WalkError.Unwrap]: a non-nil [*GitIgnoreParseErrors] with 1+ errors.
	WalkErrorKind_ParseGitIgnore
	// WalkErrorKind_FSRootNotRepo indicates that [WalkOptions.RespectGitIgnore]
	// is set but the filesystem root is not a git repository root.
	//
	// Methods returning data:
	//   - [WalkError.Path]: absolute path of the filesystem root.
	WalkErrorKind_FSRootNotRepo
	// WalkErrorKind_RootIsIgnored indicates that the walk root is excluded by
	// an ancestor .gitignore rule.
	//
	// Methods returning data:
	//   - [WalkError.Path]: absolute path of the walk root.
	//   - [WalkError.GitIgnorePattern]: the matching ignore pattern.
	WalkErrorKind_RootIsIgnored
)

// WalkError is the structured error type returned by [WalkNonDet].
type WalkError struct {
	kind WalkErrorKind
	// path is the absolute path that triggered the error.
	// Set for all kinds.
	path pathx.AbsPath
	// err is the underlying error.
	// Set for [WalkErrorKind_IOFailed] and [WalkErrorKind_ParseGitIgnore].
	err error
	// gitignorePattern is the matching ignore pattern.
	// Set for [WalkErrorKind_RootIsIgnored].
	gitignorePattern option.Option[source_code.Snippet]
}

// Kind returns the error kind.
//
// Valid for all [WalkErrorKind] values.
func (e *WalkError) Kind() WalkErrorKind {
	return e.kind
}

// Path returns the absolute path associated with the error.
//
// Valid for all [WalkErrorKind] values. The exact meaning of the path depends
// on the kind; see the per-kind documentation on [WalkErrorKind].
func (e *WalkError) Path() pathx.AbsPath {
	return e.path
}

// GitIgnorePattern returns the matching .gitignore pattern.
//
// Pre-condition: [WalkError.Kind] == [WalkErrorKind_RootIsIgnored].
func (e *WalkError) GitIgnorePattern() source_code.Snippet {
	assert.Preconditionf(e.kind == WalkErrorKind_RootIsIgnored, "GitIgnorePattern() called on WalkError kind %v", e.kind)
	pattern, ok := e.gitignorePattern.Get()
	assert.Invariant(ok, "gitignorePattern not initialized")
	return pattern
}

func (e *WalkError) Error() string {
	switch e.kind {
	case WalkErrorKind_IOFailed:
		return fmt.Sprintf("%s: %v", e.path, e.err)
	case WalkErrorKind_RootNotDir:
		return fmt.Sprintf("walk root %s is not a directory", e.path)
	case WalkErrorKind_ParseGitIgnore:
		return fmt.Sprintf("parse %s: %v", e.path, e.err)
	case WalkErrorKind_FSRootNotRepo:
		return fmt.Sprintf("filesystem root %s is not a git repository root", e.path)
	case WalkErrorKind_RootIsIgnored:
		pattern := e.GitIgnorePattern()
		return fmt.Sprintf(
			"walk root %s is excluded by ancestor .gitignore pattern %q at %s",
			e.path,
			pattern.Text,
			pattern.FilePosition(),
		)
	default:
		return assert.PanicUnknownCase[string](e.kind)
	}
}

// Unwrap returns the underlying error.
//
// For [WalkErrorKind_IOFailed], this is the underlying filesystem I/O error.
// For [WalkErrorKind_ParseGitIgnore], this is a non-nil
// [*GitIgnoreParseErrors]. For all other [WalkErrorKind] values, this returns
// nil.
func (e *WalkError) Unwrap() error {
	return e.err
}

func newWalkError(kind WalkErrorKind, path pathx.AbsPath) *WalkError {
	return &WalkError{kind, path, nil, option.None[source_code.Snippet]()}
}

func (e *WalkError) withErr(err error) *WalkError {
	assert.Preconditionf(e.err == nil, "overwriting existing error: %v", e.err)
	assert.Precondition(err != nil, "setting nil error")
	e.err = err
	return e
}

func (e *WalkError) withGitIgnorePattern(pattern source_code.Snippet) *WalkError {
	assert.Preconditionf(e.gitignorePattern.IsNone(), "overwriting existing gitignore pattern: %v", e.gitignorePattern)
	e.gitignorePattern = option.Some(pattern)
	return e
}

// GitIgnoreParseErrors is the underlying error of a [WalkErrorKind_ParseGitIgnore]
// [WalkError]. It carries every parse error from a single .gitignore file via
// [GitIgnoreParseErrors.Unwrap], and exposes the corresponding source snippets
// via [GitIgnoreParseErrors.Snippets].
type GitIgnoreParseErrors struct {
	errs     []gitignore.Error
	snippets []source_code.Snippet
}

// newGitIgnoreParseErrors collects the parse errors and their source snippets
// for inclusion in a [WalkErrorKind_ParseGitIgnore] WalkError.
//
// Pre-condition: errs is non-empty.
func newGitIgnoreParseErrors(ignoreFile pathx.RelPath, content []byte, errs []gitignore.Error) *GitIgnoreParseErrors {
	assert.Preconditionf(len(errs) > 0, "newGitIgnoreParseErrors called with no errors")
	return &GitIgnoreParseErrors{
		errs:     errs,
		snippets: gitIgnoreErrorSnippets(ignoreFile, content, errs),
	}
}

func (e *GitIgnoreParseErrors) Error() string {
	var b strings.Builder
	for i, err := range e.errs {
		if i > 0 {
			b.WriteString("; ")
		}
		pos := err.Position()
		if pos.Zero() {
			_, _ = fmt.Fprintf(&b, "%v", err.Underlying())
			continue
		}
		_, _ = fmt.Fprintf(&b, "%s: %v", pos, err.Underlying())
	}
	return b.String()
}

func (e *GitIgnoreParseErrors) Unwrap() []error {
	errs := make([]error, 0, len(e.errs))
	for _, err := range e.errs {
		errs = append(errs, err)
	}
	return errs
}

func (e *GitIgnoreParseErrors) Snippets() []source_code.Snippet {
	return slices.Clone(e.snippets)
}

func gitIgnoreErrorSnippets(ignoreFile pathx.RelPath, content []byte, errs []gitignore.Error) []source_code.Snippet {
	lines := strings.Split(string(content), "\n")
	snippets := make([]source_code.Snippet, 0, len(errs))
	for _, err := range errs {
		pos := err.Position()
		if pos.Zero() {
			continue
		}
		lineIndex := pos.Line - 1
		assert.Invariantf(0 <= lineIndex && lineIndex < len(lines), "gitignore error position %s outside source", pos)
		snippets = append(snippets, source_code.Snippet{
			Path:     ignoreFile,
			Position: source_code.NewPosition(int32(pos.Line), int32(pos.Column)),
			Text:     lines[lineIndex],
		})
	}
	return snippets
}
