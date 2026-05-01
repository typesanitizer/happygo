// Package fsx_testkit provides test helpers for fsx.FS values.
package fsx_testkit

import (
	"iter"
	"os"
	"runtime"
	"strings"

	"github.com/typesanitizer/happygo/common/check"
	"github.com/typesanitizer/happygo/common/core/pathx"
	"github.com/typesanitizer/happygo/common/core/result"
	"github.com/typesanitizer/happygo/common/errorx"
	"github.com/typesanitizer/happygo/common/fsx"
	"github.com/typesanitizer/happygo/common/syscaps"
)

// TempDirFS returns a host FS rooted at a new test temporary directory.
func TempDirFS(h check.Harness) fsx.FS {
	h.T().Helper()
	fs, err := syscaps.FS(pathx.NewAbsPath(h.T().TempDir()))
	h.NoErrorf(err, "FS(TempDir())")
	return fs
}

func NewMemFS(h check.Harness) fsx.FS {
	h.T().Helper()
	root := FakeRoot()
	fs, err := fsx.MemMap(root)
	h.NoErrorf(err, "MemMap(%q)", root)
	return fs
}

// WriteFile writes a single file, creating parent directories as needed.
func WriteFile(h check.Harness, fs fsx.FS, path pathx.RelPath, content string) {
	h.T().Helper()
	if dir, ok := path.Dir().Get(); ok {
		h.NoErrorf(fs.MkdirAll(dir, 0o755), "MkdirAll(%q)", dir)
	}
	h.NoErrorf(fs.WriteFile(path, []byte(content), 0o644), "WriteFile(%q)", path)
}

// WriteTree creates files and directories in fs from a map.
// Keys ending in "/" create directories (value must be "").
// Other keys create files with the value as content.
func WriteTree(h check.Harness, fs fsx.FS, tree map[string]string) {
	h.T().Helper()
	for path, content := range tree {
		h.Assertf(path != "", "path must not be empty")
		rel := pathx.NewRelPath(path)
		if strings.HasSuffix(path, "/") {
			h.Assertf(content == "", "directory path %q must have empty content", path)
			h.NoErrorf(fs.MkdirAll(rel, 0o755), "MkdirAll(%q)", rel)
			continue
		}
		WriteFile(h, fs, rel, content)
	}
}

// NewFaultyFS returns an fsx.FS wrapper that injects failures for configured
// operations.
func NewFaultyFS(h check.Harness, base fsx.FS, faults ...Fault) fsx.FS {
	h.T().Helper()
	return newFaultyFS(base, faults...)
}

// FaultOp identifies the filesystem operation to fail.
type FaultOp uint8

const (
	FaultOp_Stat FaultOp = iota + 1
	FaultOp_Open
)

// Fault describes one injected filesystem failure.
type Fault struct {
	Op  FaultOp
	Rel pathx.RelPath
}

// faultyFS wraps an fsx.FS and injects failures for configured operations.
type faultyFS struct {
	fsx.FS
	faults map[Fault]struct{}
}

func newFaultyFS(base fsx.FS, faults ...Fault) *faultyFS {
	faultSet := make(map[Fault]struct{}, len(faults))
	for _, f := range faults {
		faultSet[f] = struct{}{}
	}
	return &faultyFS{FS: base, faults: faultSet}
}

func (fs *faultyFS) Stat(rel pathx.RelPath, opts fsx.StatOptions) (os.FileInfo, error) {
	if fs.hasFault(FaultOp_Stat, rel) {
		return nil, injectedFSError()
	}
	return fs.FS.Stat(rel, opts)
}

func (fs *faultyFS) Open(rel pathx.RelPath) (fsx.File, error) {
	if fs.hasFault(FaultOp_Open, rel) {
		return nil, injectedFSError()
	}
	return fs.FS.Open(rel)
}

func (fs *faultyFS) ReadFile(rel pathx.RelPath) ([]byte, error) {
	if fs.hasFault(FaultOp_Open, rel) {
		return nil, injectedFSError()
	}
	return fs.FS.ReadFile(rel)
}

func (fs *faultyFS) ReadDir(rel pathx.RelPath) iter.Seq[result.Result[fsx.DirEntry]] {
	if fs.hasFault(FaultOp_Open, rel) {
		return func(yield func(result.Result[fsx.DirEntry]) bool) {
			yield(result.Failure[fsx.DirEntry](injectedFSError()))
		}
	}
	return fs.FS.ReadDir(rel)
}

func (fs *faultyFS) hasFault(op FaultOp, rel pathx.RelPath) bool {
	_, hit := fs.faults[Fault{Op: op, Rel: rel}]
	return hit
}

func injectedFSError() error {
	return errorx.New("nostack", "injected fs error")
}

func FakeRoot() pathx.AbsPath {
	if runtime.GOOS == "windows" {
		return pathx.NewAbsPath(`C:\virtual-root`)
	}
	return pathx.NewAbsPath("/virtual-root")
}
