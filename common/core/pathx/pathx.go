// Package pathx provides filepath utilities.
package pathx

import (
	"iter"
	"os"
	"path/filepath"
	"strings"

	"github.com/typesanitizer/happygo/common/assert"
	"github.com/typesanitizer/happygo/common/core/option"
	"github.com/typesanitizer/happygo/common/core/pathx/internal/winpath"
)

type AbsPath struct {
	value string
}

func NewAbsPath(path string) AbsPath {
	assert.Preconditionf(path != "", "path is empty")
	assert.Preconditionf(filepath.IsAbs(path), "path is not absolute: %q", path)
	return AbsPath{path}
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

func (p AbsPath) MkdirTemp(pattern string) (AbsPath, error) {
	assert.Preconditionf(!strings.ContainsAny(pattern, "/\\"), "pattern contains path separator: %q", pattern)
	dir, err := os.MkdirTemp(p.value, pattern)
	if err != nil {
		return AbsPath{}, err
	}
	return NewAbsPath(dir), nil
}

func (p AbsPath) RemoveAll() error {
	return os.RemoveAll(p.value)
}

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
	if winpath.IsWindowsStyleAbsPath(p.value) {
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

func (p AbsPath) JoinComponents(pathElems ...string) AbsPath {
	parts := make([]string, 0, len(pathElems)+1)
	parts = append(parts, p.value)
	parts = append(parts, pathElems...)
	return NewAbsPath(filepath.Join(parts...))
}

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

type RelPath struct {
	value string
}

func NewRelPath(path string) RelPath {
	assert.Preconditionf(path != "", "path is empty")
	assert.Preconditionf(!filepath.IsAbs(path), "path is not relative: %q", path)
	return RelPath{path}
}

func (p RelPath) String() string {
	return p.value
}

func (p RelPath) Components() iter.Seq[string] {
	return func(yield func(string) bool) {
		start := 0
		for i := 0; i <= len(p.value); i++ {
			if i < len(p.value) && !isPathSeparator(p.value[i]) {
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

func isPathSeparator(b byte) bool {
	if filepath.Separator == '\\' {
		return b == '\\' || b == '/'
	}
	return b == '/'
}

type RootRelPath struct {
	root  AbsPath
	value RelPath
}

func NewRootRelPath(root AbsPath, subpath RelPath) RootRelPath {
	assert.Preconditionf(root.LexicallyContains(subpath), "subpath %q escapes root %q", subpath.value, root.value)
	return RootRelPath{root: root, value: subpath}
}

func (p RootRelPath) String() string {
	return p.value.value
}

func (p RootRelPath) Resolve() AbsPath {
	return p.root.Join(p.value)
}
