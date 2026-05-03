// Copyright 2026 Varun Gandhi
//
// SPDX-License-Identifier: PolyForm-Noncommercial-1.0.0

// Package fsx provides a rooted filesystem wrapper over [afero.Fs] that
// operates on [RelPath] values anchored at a repo-root [AbsPath].
//
// The goal is to keep pure path operations in [pathx] while routing all
// filesystem effects through this package, so typed paths stay internal and
// string conversion is confined to the filesystem boundary.
package fsx

import (
	"io"
	iofs "io/fs" //nolint:depguard // fsx is the designated wrapper
	"iter"
	"os"

	"github.com/spf13/afero" //nolint:depguard // fsx is the designated wrapper
	"github.com/typesanitizer/happygo/common/fsx/fsx_name"

	"github.com/typesanitizer/happygo/common/assert"
	"github.com/typesanitizer/happygo/common/core/pathx"
	"github.com/typesanitizer/happygo/common/core/result"
	"github.com/typesanitizer/happygo/common/errorx"
	"github.com/typesanitizer/happygo/common/internal/constants"
	"github.com/typesanitizer/happygo/common/iterx"
)

// ErrNotExist is [fs.ErrNotExist], re-exported so callers need not import io/fs.
var ErrNotExist = iofs.ErrNotExist

// File is an open file handle returned by [FS.Open] and similar methods.
// It is an alias for [afero.File] so callers need not import afero directly.
type File = afero.File

type OpenMode int

const (
	OpenMode_ReadOnly OpenMode = iota + 1
	OpenMode_WriteOnly
	OpenMode_ReadWrite
)

type OpenOptions struct {
	Mode OpenMode
}

// DirEntry is a single entry returned by [FS.ReadDir].
type DirEntry interface {
	// BaseName returns the basename of the entry as a validated [Name].
	BaseName() Name
	IsDir() bool
	Info() (os.FileInfo, error)
}

type dirEntry struct {
	entry iofs.DirEntry
}

func (e dirEntry) BaseName() Name {
	n, err := fsx_name.Parse(e.entry.Name())
	assert.Invariantf(err == nil, "filesystem returned invalid Name(): %v", err)
	return n
}

func (e dirEntry) IsDir() bool {
	return e.entry.IsDir()
}

func (e dirEntry) Info() (os.FileInfo, error) {
	return e.entry.Info()
}

type BaseFS interface {
	afero.Fs
	afero.Lstater
}

// FS is a rooted filesystem. All methods operate on paths relative to Root().
type FS interface {
	Root() pathx.AbsPath
	ReadDir(rel pathx.RelPath) iter.Seq[result.Result[DirEntry]]
	Open(rel pathx.RelPath, opts OpenOptions) (File, error)
	ReadFile(rel pathx.RelPath) ([]byte, error)
	WriteFile(rel pathx.RelPath, data []byte, perm os.FileMode) error
	MkdirAll(rel pathx.RelPath, perm os.FileMode) error
	MkdirTemp(dir pathx.RelPath, pattern string) (pathx.RelPath, error)
	RemoveAll(rel pathx.RelPath) error
	Stat(rel pathx.RelPath, opts StatOptions) (os.FileInfo, error)
}

// rootedFS is the standard FS implementation backed by an afero filesystem.
type rootedFS struct {
	// Stored separately because afero.BasePathFs does not expose its configured
	// root path back to callers, but fsx needs to provide Root().
	root pathx.AbsPath
	base BaseFS
}

// MemMap returns an in-memory FS rooted at root.
func MemMap(root pathx.AbsPath) (FS, error) {
	backing := afero.NewMemMapFs()
	if err := backing.MkdirAll(root.String(), 0o755); err != nil {
		return nil, errorx.Wrapf("+stacks", err, "create fs root %s", root)
	}
	base, ok := backing.(BaseFS)
	assert.Invariantf(ok, "NewMemMapFs return value should implement BaseFS, but got type %T", backing)
	return NewRootedFS(root, base)
}

// NewRootedFS returns an FS rooted at root and backed by backing.
//
// Pre-condition: root must already exist in backing and be a directory.
func NewRootedFS(root pathx.AbsPath, backing BaseFS) (FS, error) {
	info, err := backing.Stat(root.String())
	if err != nil {
		return nil, errorx.Wrapf("+stacks", err, "stat fs root %s", root)
	}
	if !info.IsDir() {
		return nil, errorx.Newf("nostack", "fs root %s is not a directory", root)
	}

	rootedBase, ok := afero.NewBasePathFs(backing, root.String()).(BaseFS)
	assert.Invariantf(ok, "NewBasePathFs return value should implement BaseFS, but got type %T", backing)
	return rootedFS{root: root, base: rootedBase}, nil
}

// Root returns the absolute path this FS is rooted at.
func (fs rootedFS) Root() pathx.AbsPath {
	return fs.root
}

// ReadDir iterates over directory entries at the given root-relative path.
//
// Errors produced mid-iteration are surfaced as [Failure] elements
// rather than being returned eagerly. Callers should stop iterating on the
// first failure.
func (fs rootedFS) ReadDir(rel pathx.RelPath) iter.Seq[result.Result[DirEntry]] {
	return iterx.Map(iterx.Unbatch(fs.readDirBatches(rel)), func(entryRes result.Result[iofs.DirEntry]) result.Result[DirEntry] {
		entry, err := entryRes.Get()
		if err != nil {
			return result.Failure[DirEntry](err)
		}
		return result.Success[DirEntry](dirEntry{entry: entry})
	})
}

func (fs rootedFS) readDirBatches(rel pathx.RelPath) iter.Seq[result.Result[[]iofs.DirEntry]] {
	return func(yield func(result.Result[[]iofs.DirEntry]) bool) {
		f, err := fs.base.Open(rel.String())
		if err != nil {
			yield(result.Failure[[]iofs.DirEntry](err))
			return
		}
		defer func() {
			_ = f.Close()
		}()

		rdf, ok := f.(iofs.ReadDirFile)
		assert.Invariantf(ok, "open(%q) returned %T, want fs.ReadDirFile", rel, f)

		for {
			entries, err := rdf.ReadDir(constants.ReadDirBatchSize)
			if len(entries) > 0 {
				if !yield(result.Success(entries)) {
					return
				}
			}
			// For n > 0, ReadDirFile guarantees that an empty batch comes with a
			// non-nil error. Yield any entries before inspecting err so a final
			// short batch is not dropped if an implementation returns it with EOF,
			// then stop immediately on EOF rather than calling ReadDir again.
			switch err {
			case nil:
				assert.Invariantf(len(entries) > 0,
					"ReadDir(%q) returned an empty batch without EOF", rel)
			case io.EOF:
				return
			default:
				yield(result.Failure[[]iofs.DirEntry](err))
				return
			}
		}
	}
}

// Open opens the file at the given root-relative path.
func (fs rootedFS) Open(rel pathx.RelPath, opts OpenOptions) (File, error) {
	return fs.base.OpenFile(rel.String(), openFlags(opts.Mode), 0)
}

func openFlags(mode OpenMode) int {
	switch mode {
	case OpenMode_ReadOnly:
		return os.O_RDONLY
	case OpenMode_WriteOnly:
		return os.O_WRONLY
	case OpenMode_ReadWrite:
		return os.O_RDWR
	default:
		return assert.PanicUnknownCase[int](mode)
	}
}

// ReadFile reads the file at the given root-relative path.
func (fs rootedFS) ReadFile(rel pathx.RelPath) ([]byte, error) {
	return afero.ReadFile(fs.base, rel.String())
}

// WriteFile writes data to the file at the given root-relative path.
func (fs rootedFS) WriteFile(rel pathx.RelPath, data []byte, perm os.FileMode) error {
	return afero.WriteFile(fs.base, rel.String(), data, perm)
}

// MkdirAll creates the directory at the given root-relative path along with
// any necessary parents.
func (fs rootedFS) MkdirAll(rel pathx.RelPath, perm os.FileMode) error {
	return fs.base.MkdirAll(rel.String(), perm)
}

// MkdirTemp creates a new temporary directory inside dir (root-relative)
// whose name begins with pattern, and returns the resulting root-relative path.
//
// Pre-condition: pattern is non-empty and contains no path separators.
func (fs rootedFS) MkdirTemp(dir pathx.RelPath, pattern string) (pathx.RelPath, error) {
	assert.Preconditionf(pattern != "", "pattern is empty")
	assert.Preconditionf(!pathx.HasPathSeparators(pattern), "pattern contains path separator: %q", pattern)
	// afero.TempDir returns filepath.Join(dir, pattern+rand) relative to fs.base,
	// so the returned path is already root-relative rather than just a basename.
	tmpDir, err := afero.TempDir(fs.base, dir.String(), pattern)
	if err != nil {
		return pathx.RelPath{}, err
	}
	return pathx.NewRelPath(tmpDir), nil
}

// RemoveAll removes the file or directory at the given root-relative path
// along with any children it contains.
func (fs rootedFS) RemoveAll(rel pathx.RelPath) error {
	return fs.base.RemoveAll(rel.String())
}
