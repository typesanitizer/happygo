// Package fsx_testkit provides test helpers for fsx.FS values.
package fsx_testkit

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/afero" //nolint:depguard // fsx_testkit needs to build test backing filesystems.

	"github.com/typesanitizer/happygo/common/assert"
	"github.com/typesanitizer/happygo/common/check"
	. "github.com/typesanitizer/happygo/common/core"
	"github.com/typesanitizer/happygo/common/errorx"
	"github.com/typesanitizer/happygo/common/fsx"
)

func NewMemFS(h check.Harness, tree map[string]string) fsx.FS {
	h.T().Helper()
	root := FakeRoot()
	fs, err := fsx.MemMap(root)
	h.NoErrorf(err, "MemMap(%q)", root)
	WriteTree(h, fs, tree)
	return fs
}

// WriteTree creates files and directories in fs from a map.
// Keys ending in "/" create directories (value must be "").
// Other keys create files with the value as content.
func WriteTree(h check.Harness, fs fsx.FS, tree map[string]string) {
	h.T().Helper()
	for path, content := range tree {
		h.Assertf(path != "", "path must not be empty")
		rel := NewRelPath(path)
		if strings.HasSuffix(path, "/") {
			h.Assertf(content == "", "directory path %q must have empty content", path)
			h.NoErrorf(fs.MkdirAll(rel, 0o755), "MkdirAll(%q)", rel)
			continue
		}
		if dir, ok := rel.Dir().Get(); ok {
			h.NoErrorf(fs.MkdirAll(dir, 0o755), "MkdirAll(%q)", dir)
		}
		h.NoErrorf(fs.WriteFile(rel, []byte(content), 0o644), "WriteFile(%q)", rel)
	}
}

// NewFaultyFS returns an in-memory fsx.FS rooted at root that injects
// failures for configured operations.
func NewFaultyFS(h check.Harness, root AbsPath, tree map[string]string, faults ...Fault) fsx.FS {
	h.T().Helper()
	backing, ok := afero.NewMemMapFs().(fsx.BaseFS)
	h.Assertf(ok, "NewMemMapFs() = %T, want fsx.BaseFS", backing)
	h.NoErrorf(backing.MkdirAll(root.String(), 0o755), "MkdirAll(%q)", root)
	memMapFS, err := fsx.NewRootedFS(root, backing)
	h.NoErrorf(err, "NewRootedFS(%q)", root)
	WriteTree(h, memMapFS, tree)
	fs, err := fsx.NewRootedFS(root, newFaultyBaseFS(backing, root, faults...))
	h.NoErrorf(err, "NewRootedFS(%q, FaultyFS)", root)
	return fs
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
	Rel RelPath
}

// faultyFS wraps an fsx.BaseFS and injects failures for configured operations.
type faultyFS struct {
	fsx.BaseFS
	root   AbsPath
	faults map[Fault]struct{}
}

func newFaultyBaseFS(base fsx.BaseFS, root AbsPath, faults ...Fault) *faultyFS {
	faultSet := make(map[Fault]struct{}, len(faults))
	for _, f := range faults {
		faultSet[f] = struct{}{}
	}
	return &faultyFS{BaseFS: base, root: root, faults: faultSet}
}

func (fs *faultyFS) Stat(absPath string) (os.FileInfo, error) {
	if fs.hasFault(FaultOp_Stat, absPath) {
		return nil, injectedFSError()
	}
	return fs.BaseFS.Stat(absPath)
}

func (fs *faultyFS) LstatIfPossible(absPath string) (os.FileInfo, bool, error) {
	if fs.hasFault(FaultOp_Stat, absPath) {
		return nil, false, injectedFSError()
	}
	return fs.BaseFS.LstatIfPossible(absPath)
}

func (fs *faultyFS) Open(absPath string) (fsx.File, error) {
	if fs.hasFault(FaultOp_Open, absPath) {
		return nil, injectedFSError()
	}
	return fs.BaseFS.Open(absPath)
}

func (fs *faultyFS) hasFault(op FaultOp, absPath string) bool {
	rel, err := filepath.Rel(fs.root.String(), absPath)
	assert.Invariantf(err == nil, "filepath.Rel(%q, %q): %v", fs.root, absPath, err)
	_, hit := fs.faults[Fault{Op: op, Rel: NewRelPath(rel)}]
	return hit
}

func injectedFSError() error {
	return errorx.New("nostack", "injected fs error")
}

func FakeRoot() AbsPath {
	if runtime.GOOS == "windows" {
		return NewAbsPath(`C:\virtual-root`)
	}
	return NewAbsPath("/virtual-root")
}
