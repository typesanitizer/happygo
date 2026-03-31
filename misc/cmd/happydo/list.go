package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strings"

	. "github.com/typesanitizer/happygo/common/core"
	"github.com/typesanitizer/happygo/common/errorx"
	"github.com/typesanitizer/happygo/common/logx"
	"github.com/typesanitizer/happygo/misc/internal/config"
)

func loadWorkspaceConfig(repoRoot AbsPath) (_ config.WorkspaceConfig, retErr error) {
	path := repoRoot.JoinComponents("misc", "repo-configuration.json").String()
	f, err := os.Open(path)
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

// Workspace provides operations over the repository root using the repo configuration.
type Workspace struct {
	RepoRoot AbsPath
	Config   config.WorkspaceConfig
}

func newWorkspaceFromGit() (Workspace, error) {
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return Workspace{}, errorx.Wrapf("nostack", err, "determine git repository root")
	}
	repoRoot := NewAbsPath(strings.TrimSpace(string(out)))
	wsConfig, err := loadWorkspaceConfig(repoRoot)
	return Workspace{RepoRoot: repoRoot, Config: wsConfig}, err
}

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
		if _, err := fmt.Fprintln(out, f); err != nil {
			return err
		}
	}
	return nil
}

func (w Workspace) goModules(logger *logx.Logger, provenance ListProvenance) ([]string, error) {
	entries, err := os.ReadDir(w.RepoRoot.String())
	if err != nil {
		return nil, errorx.Wrapf("+stacks", err, "read directory %s", w.RepoRoot.String())
	}

	var folders []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if _, err := os.Stat(w.RepoRoot.JoinComponents(name, "go.mod").String()); err != nil {
			if !os.IsNotExist(err) {
				logger.Warn("stat go.mod", "dir", name, "err", err)
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
	sort.Strings(folders) // for determinism

	if len(folders) == 0 {
		return nil, errorx.Newf("nostack", "no Go modules found matching filter")
	}
	return folders, nil
}
