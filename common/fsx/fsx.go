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

	"github.com/typesanitizer/happygo/common/assert"
	. "github.com/typesanitizer/happygo/common/core"
	"github.com/typesanitizer/happygo/common/core/pathx"
	"github.com/typesanitizer/happygo/common/errorx"
	"github.com/typesanitizer/happygo/common/iterx"
)

const readDirBatchSize = 32

// File is an open file handle returned by [FS.Open] and similar methods.
// It is an alias for [afero.File] so callers need not import afero directly.
type File = afero.File

// DirEntry is a single entry returned by [FS.ReadDir].
type DirEntry interface {
	// BaseName returns just the basename component of the entry.
	BaseName() string
	IsDir() bool
	Info() (os.FileInfo, error)
}

type dirEntry struct {
	entry iofs.DirEntry
}

func (e dirEntry) BaseName() string {
	return e.entry.Name()
}

func (e dirEntry) IsDir() bool {
	return e.entry.IsDir()
}

func (e dirEntry) Info() (os.FileInfo, error) {
	return e.entry.Info()
}

// FS is a rooted filesystem wrapper. All methods operate on paths relative
// to Root().
type FS struct {
	// Stored separately because afero.BasePathFs does not expose its configured
	// root path back to callers, but fsx needs to provide Root().
	root AbsPath
	base afero.Fs
}

// OS returns an FS backed by the host operating system rooted at root.
func OS(root AbsPath) (FS, error) {
	return newRootedFS(root, afero.NewOsFs())
}

// MemMap returns an in-memory FS rooted at root.
func MemMap(root AbsPath) (FS, error) {
	backing := afero.NewMemMapFs()
	if err := backing.MkdirAll(root.String(), 0o755); err != nil {
		return FS{}, errorx.Wrapf("+stacks", err, "create fs root %s", root)
	}
	return newRootedFS(root, backing)
}

func newRootedFS(root AbsPath, backing afero.Fs) (FS, error) {
	info, err := backing.Stat(root.String())
	if err != nil {
		return FS{}, errorx.Wrapf("+stacks", err, "stat fs root %s", root)
	}
	if !info.IsDir() {
		return FS{}, errorx.Newf("nostack", "fs root %s is not a directory", root)
	}
	return FS{root: root, base: afero.NewBasePathFs(backing, root.String())}, nil
}

// Root returns the absolute path this FS is rooted at.
func (fs FS) Root() AbsPath {
	return fs.root
}

// Stat returns file info for the given root-relative path.
func (fs FS) Stat(rel RelPath) (os.FileInfo, error) {
	return fs.base.Stat(rel.String())
}

// ReadDir iterates over directory entries at the given root-relative path.
//
// Errors produced mid-iteration are surfaced as [Failure] elements
// rather than being returned eagerly. Callers should stop iterating on the
// first failure.
func (fs FS) ReadDir(rel RelPath) iter.Seq[Result[DirEntry]] {
	return iterx.Map(iterx.Unbatch(fs.readDirBatches(rel)), func(entryRes Result[iofs.DirEntry]) Result[DirEntry] {
		entry, err := entryRes.Get()
		if err != nil {
			return Failure[DirEntry](err)
		}
		return Success[DirEntry](dirEntry{entry: entry})
	})
}

func (fs FS) readDirBatches(rel RelPath) iter.Seq[Result[[]iofs.DirEntry]] {
	return func(yield func(Result[[]iofs.DirEntry]) bool) {
		f, err := fs.base.Open(rel.String())
		if err != nil {
			yield(Failure[[]iofs.DirEntry](err))
			return
		}
		defer func() {
			_ = f.Close()
		}()

		rdf, ok := f.(iofs.ReadDirFile)
		assert.Invariantf(ok, "open(%q) returned %T, want fs.ReadDirFile", rel, f)

		for {
			entries, err := rdf.ReadDir(readDirBatchSize)
			if len(entries) > 0 {
				if !yield(Success(entries)) {
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
				yield(Failure[[]iofs.DirEntry](err))
				return
			}
		}
	}
}

// Open opens the file at the given root-relative path for reading.
func (fs FS) Open(rel RelPath) (File, error) {
	return fs.base.Open(rel.String())
}

// ReadFile reads the file at the given root-relative path.
func (fs FS) ReadFile(rel RelPath) ([]byte, error) {
	return afero.ReadFile(fs.base, rel.String())
}

// WriteFile writes data to the file at the given root-relative path.
func (fs FS) WriteFile(rel RelPath, data []byte, perm os.FileMode) error {
	return afero.WriteFile(fs.base, rel.String(), data, perm)
}

// MkdirAll creates the directory at the given root-relative path along with
// any necessary parents.
func (fs FS) MkdirAll(rel RelPath, perm os.FileMode) error {
	return fs.base.MkdirAll(rel.String(), perm)
}

// MkdirTemp creates a new temporary directory inside dir (root-relative)
// whose name begins with pattern, and returns the resulting root-relative path.
//
// Pre-condition: pattern is non-empty and contains no path separators.
func (fs FS) MkdirTemp(dir RelPath, pattern string) (RelPath, error) {
	assert.Preconditionf(pattern != "", "pattern is empty")
	assert.Preconditionf(!pathx.HasPathSeparators(pattern), "pattern contains path separator: %q", pattern)
	// afero.TempDir returns filepath.Join(dir, pattern+rand) relative to fs.base,
	// so the returned path is already root-relative rather than just a basename.
	tmpDir, err := afero.TempDir(fs.base, dir.String(), pattern)
	if err != nil {
		return RelPath{}, err
	}
	return NewRelPath(tmpDir), nil
}

// RemoveAll removes the file or directory at the given root-relative path
// along with any children it contains.
func (fs FS) RemoveAll(rel RelPath) error {
	return fs.base.RemoveAll(rel.String())
}
