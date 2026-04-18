// Package pathx provides typed path wrappers for host platform paths.
//
// These types improve code clarity and catch potential bugs (e.g. accidentally
// passing a relative path where an absolute one is expected). They are not
// a security mechanism; for sandboxed filesystem access, use [os.Root].
package pathx

import (
	"iter"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/typesanitizer/happygo/common/assert"
	"github.com/typesanitizer/happygo/common/core/option"
)

// AbsPath carries an absolute path that has gone through [LexicallyNormalize].
//
// It is guaranteed to be non-empty.
type AbsPath struct {
	value string
}

// NewAbsPath creates an AbsPath from an already-absolute path string.
//
// Pre-condition: path is non-empty and absolute per [filepath.IsAbs].
func NewAbsPath(path string) AbsPath {
	assert.Preconditionf(path != "", "path is empty")
	assert.Preconditionf(filepath.IsAbs(path), "path is not absolute: %q", path)
	return AbsPath{LexicallyNormalize(path)}
}

func (p AbsPath) String() string {
	return p.value
}

func (p AbsPath) Dir() AbsPath {
	return NewAbsPath(filepath.Dir(p.value))
}

func (p AbsPath) Split() (AbsPath, string) {
	dir, file := filepath.Split(p.value)
	return NewAbsPath(dir), file
}

// ResolveAbsPath resolves a possibly-relative path to an AbsPath
// using [filepath.Abs].
//
// Pre-condition: path is non-empty.
func ResolveAbsPath(path string) (AbsPath, error) {
	assert.Preconditionf(path != "", "path is empty")
	absPath, err := filepath.Abs(path)
	if err != nil {
		return AbsPath{}, err
	}
	return NewAbsPath(absPath), nil
}

func (p AbsPath) MkdirAll(perm os.FileMode) error {
	return os.MkdirAll(p.value, perm)
}

func (p AbsPath) RemoveAll() error {
	return os.RemoveAll(p.value)
}

// TODO(varun): Replace with something that returns iter.Seq[Result[os.DirEntry]].
func (p AbsPath) ReadDir() ([]os.DirEntry, error) {
	return os.ReadDir(p.value)
}

func (p AbsPath) Stat() (os.FileInfo, error) {
	return os.Stat(p.value)
}

func (p AbsPath) ReadFile() ([]byte, error) {
	return os.ReadFile(p.value)
}

func (p AbsPath) WriteFile(data []byte, perm os.FileMode) error {
	return os.WriteFile(p.value, data, perm)
}

// LexicallyContains reports whether child is lexically contained within p.
func (p AbsPath) LexicallyContains(child RelPath) bool {
	if runtime.GOOS == "windows" {
		return p.lexicallyContainsSlow(child)
	}
	return child.lexicallyContainsUnix()
}

func (p AbsPath) lexicallyContainsSlow(child RelPath) bool {
	rel, err := filepath.Rel(p.value, filepath.Join(p.value, child.value))
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func (p AbsPath) Join(rel RelPath) AbsPath {
	return NewAbsPath(filepath.Join(p.value, rel.value))
}

// AppendExtension returns p with ext appended.
//
// Pre-conditions:
// 1. p must be non-empty.
// 2. p does not end with a path separator (i.e. p must be a valid file path).
func (p AbsPath) AppendExtension(ext string) AbsPath {
	if len(p.value) == 0 {
		assert.Precondition(false, "empty path")
	} else {
		lastCharStr := p.value[len(p.value)-1:]
		assert.Preconditionf(!HasPathSeparators(lastCharStr),
			"path %q ends with a path separator; so it's not a valid file path", p.value)
	}
	return NewAbsPath(p.value + ext)
}

// JoinComponents joins individual path components onto p.
//
// Pre-condition: no element contains a path separator.
func (p AbsPath) JoinComponents(pathElems ...string) AbsPath {
	parts := make([]string, 0, len(pathElems)+1)
	parts = append(parts, p.value)
	for _, elem := range pathElems {
		assert.Preconditionf(!HasPathSeparators(elem), "path element contains separator: %q", elem)
		parts = append(parts, elem)
	}
	return NewAbsPath(filepath.Join(parts...))
}

// MakeRelativeTo is the equivalent of filepath.Rel with typed paths.
//
// If the receiver and the root are the same, then
// Some(RootRelPath{root, Rel: "."}) will be returned.
func (p AbsPath) MakeRelativeTo(root AbsPath) option.Option[RootRelPath] {
	rel, err := filepath.Rel(root.value, p.value)
	if err != nil {
		return option.None[RootRelPath]()
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return option.None[RootRelPath]()
	}
	return option.Some(NewRootRelPath(root, NewRelPath(rel)))
}

// RelPath carries a relative path that has gone through [LexicallyNormalize].
//
// It is guaranteed to be non-empty.
type RelPath struct {
	value string
}

// Dot returns a relative path '.'.
func Dot() RelPath {
	return RelPath{"."}
}

// NewRelPath creates a RelPath from a relative path string.
//
// Pre-condition: path is non-empty and not absolute per [filepath.IsAbs].
func NewRelPath(path string) RelPath {
	assert.Preconditionf(path != "", "path is empty")
	assert.Preconditionf(!filepath.IsAbs(path), "path is not relative: %q", path)
	return RelPath{LexicallyNormalize(path)}
}

// String is guaranteed to be "." if a relative path for the current directory.
func (p RelPath) String() string {
	return p.value
}

// Dir returns the parent directory of p, or None if p is ".".
func (p RelPath) Dir() option.Option[RelPath] {
	parent := filepath.Dir(p.value)
	if parent == p.value || parent == "." {
		return option.None[RelPath]()
	}
	return option.Some(NewRelPath(parent))
}

func (p RelPath) Join(rel RelPath) RelPath {
	return NewRelPath(filepath.Join(p.value, rel.value))
}

// RelativeTo returns p expressed as a relative path from base.
//
// Pre-condition: base is an ancestor of p, or equal to p.
// In particular, base == "." returns p unchanged, and base == p returns ".".
func (p RelPath) RelativeTo(base RelPath) RelPath {
	if base.value == "." {
		return p
	}
	suffix, ok := strings.CutPrefix(p.value, base.value)
	assert.Preconditionf(ok, "base %q is not an ancestor of %q", base.value, p.value)
	if suffix == "" {
		return Dot()
	}
	assert.Preconditionf(IsPathSeparator(suffix[0]), "base %q is not an ancestor of %q", base.value, p.value)
	return RelPath{suffix[1:]}
}

// JoinComponents joins individual path components onto p.
//
// Pre-condition: all elements are non-empty and do not contain a path separator.
func (p RelPath) JoinComponents(pathElems ...string) RelPath {
	parts := make([]string, 0, len(pathElems)+1)
	parts = append(parts, p.value)
	for _, elem := range pathElems {
		assert.Preconditionf(!HasPathSeparators(elem), "path element contains separator: %q", elem)
		parts = append(parts, elem)
	}
	return NewRelPath(filepath.Join(parts...))
}

// Ancestors returns an iterator over p's ancestor relative paths,
// in shortest-first order. The receiver itself is not yielded.
//
// For example, "a/b/c" yields "a" then "a/b". A path of "."
// or a single-component path yields nothing.
func (p RelPath) Ancestors() iter.Seq[RelPath] {
	return func(yield func(RelPath) bool) {
		if p.value == "." {
			return
		}
		n := len(p.value)
		for i := range n {
			if !IsPathSeparator(p.value[i]) {
				continue
			}
			// A trailing separator means the ancestor would equal the
			// receiver semantically; stop here.
			if i == n-1 {
				return
			}
			if !yield(RelPath{p.value[:i]}) {
				return
			}
		}
	}
}

func (p RelPath) Components() iter.Seq[string] {
	if runtime.GOOS == "windows" {
		return p.componentsWindows()
	}
	return p.componentsUnix()
}

func (p RelPath) componentsUnix() iter.Seq[string] {
	return func(yield func(string) bool) {
		start := 0
		for i := 0; i <= len(p.value); i++ {
			if i < len(p.value) && p.value[i] != '/' {
				continue
			}
			if start < i {
				if !yield(p.value[start:i]) {
					return
				}
			}
			start = i + 1
		}
	}
}

func (p RelPath) componentsWindows() iter.Seq[string] {
	return func(yield func(string) bool) {
		start := 0
		for i := 0; i <= len(p.value); i++ {
			if i < len(p.value) && p.value[i] != '/' && p.value[i] != '\\' {
				continue
			}
			if start < i {
				if !yield(p.value[start:i]) {
					return
				}
			}
			start = i + 1
		}
	}
}

func (p RelPath) lexicallyContainsUnix() bool {
	depth := 0
	for component := range p.Components() {
		switch component {
		case ".":
			continue
		case "..":
			if depth == 0 {
				return false
			}
			depth--
		default:
			depth++
		}
	}
	return true
}

// HasPathSeparators reports whether s contains any path separators.
func HasPathSeparators(s string) bool {
	for i := range len(s) {
		if IsPathSeparator(s[i]) {
			return true
		}
	}
	return false
}

type RootRelPath struct {
	root  AbsPath
	value RelPath
}

// NewRootRelPath creates a RootRelPath anchored at root.
//
// Pre-condition: subpath does not escape root (per [AbsPath.LexicallyContains]).
func NewRootRelPath(root AbsPath, subpath RelPath) RootRelPath {
	assert.Preconditionf(root.LexicallyContains(subpath), "subpath %q escapes root %q", subpath.value, root.value)
	return RootRelPath{root: root, value: subpath}
}

func (p RootRelPath) String() string {
	return p.value.value
}

func (p RootRelPath) AsAbsPath() AbsPath {
	return p.root.Join(p.value)
}

// Rel returns the anchored relative portion of p, discarding the root.
func (p RootRelPath) Rel() RelPath {
	return p.value
}
