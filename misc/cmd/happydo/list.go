package main

import (
	"fmt"
	"io"
	"slices"

	. "github.com/typesanitizer/happygo/common/core"
	"github.com/typesanitizer/happygo/common/core/pathx"
	"github.com/typesanitizer/happygo/common/errorx"
	"github.com/typesanitizer/happygo/common/fsx"
	"github.com/typesanitizer/happygo/common/logx"
	"github.com/typesanitizer/happygo/misc/internal/config"
)

func loadWorkspaceConfig(repoFS fsx.FS) (_ config.WorkspaceConfig, retErr error) {
	path := NewRelPath("misc/repo-configuration.json")
	f, err := repoFS.Open(path)
	if err != nil {
		return config.WorkspaceConfig{}, errorx.Wrapf("+stacks", err, "open %s", path)
	}
	defer func() {
		if closeErr := f.Close(); closeErr != nil && retErr == nil {
			retErr = errorx.Wrapf("+stacks", closeErr, "close %s", path)
		}
	}()
	wsConfig, err := config.Load(f)
	if err != nil {
		return config.WorkspaceConfig{}, errorx.Wrapf("nostack", err, "load %s", path)
	}
	return wsConfig, nil
}

type ListType int

const (
	ListType_GoModules ListType = iota + 1
)

type ListProvenance int

const (
	ListProvenance_All ListProvenance = iota + 1
	ListProvenance_FirstParty
	ListProvenance_Forked
)

type ListOptions struct {
	Type       ListType
	Provenance ListProvenance
}

// List writes folder names matching the options, one per line.
func (w Workspace) List(logger *logx.Logger, out io.Writer, opts ListOptions) error {
	switch opts.Type {
	case ListType_GoModules:
		return w.listGoModules(logger, out, opts.Provenance)
	default:
		return errorx.Newf("nostack", "unknown list type %d", opts.Type)
	}
}

func (w Workspace) listGoModules(logger *logx.Logger, out io.Writer, provenance ListProvenance) error {
	folders, err := w.goModules(logger, provenance)
	if err != nil {
		return err
	}
	for _, f := range folders {
		if _, err := fmt.Fprintln(out, f.String()); err != nil {
			return err
		}
	}
	return nil
}

func (w Workspace) goModules(logger *logx.Logger, provenance ListProvenance) ([]fsx.Name, error) {
	rootRel := pathx.Dot()
	var folders []fsx.Name
	for entryRes := range w.FS.ReadDir(rootRel) {
		entry, err := entryRes.Get()
		if err != nil {
			return nil, errorx.Wrapf("+stacks", err, "read directory %s", w.FS.Root())
		}
		if !entry.IsDir() {
			continue
		}
		name := entry.BaseName()
		goModRel := rootRel.JoinComponents(name.String(), "go.mod")
		if _, statErr := w.FS.Stat(goModRel, fsx.StatOptions{FollowFinalSymlink: true, OnErrorTraverseParents: false}); statErr != nil {
			if !errorx.GetRootCauseAsValue(statErr, fsx.ErrNotExist) {
				logger.Warn("stat go.mod", "dir", name, "err", statErr)
			}
			continue
		}
		_, isForked := w.Config.ForkedFolders[name]
		switch provenance {
		case ListProvenance_All:
			folders = append(folders, name)
		case ListProvenance_FirstParty:
			if !isForked {
				folders = append(folders, name)
			}
		case ListProvenance_Forked:
			if isForked {
				folders = append(folders, name)
			}
		}
	}
	slices.SortFunc(folders, fsx.Name.Compare) // for determinism

	if len(folders) == 0 {
		return nil, errorx.Newf("nostack", "no Go modules found matching filter")
	}
	return folders, nil
}
