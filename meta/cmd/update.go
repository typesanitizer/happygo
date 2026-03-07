package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/typesanitizer/happygo/common/cmdx"
	. "github.com/typesanitizer/happygo/common/core"
	"github.com/typesanitizer/happygo/common/errorx"
	"github.com/typesanitizer/happygo/common/logx"
	"github.com/typesanitizer/happygo/meta/internal/config"
)

func runUpdate(ctx logx.LogCtx, repoRoot string, localBranch string, projects []string) error {
	wsConfig, err := loadWorkspaceConfig(repoRoot)
	if err != nil {
		return err
	}

	for _, project := range projects {
		upstream, err := wsConfig.UpstreamForProject(localBranch, project)
		if err != nil {
			return err
		}
		upstreamURL := fmt.Sprintf("https://github.com/%s.git", upstream.GitHubRepo)
		ctx.Info("running subtree pull", "project", project, "upstream", upstreamURL, "upstream_branch", upstream.Branch)
		subtreePullCmd := cmdx.Cmd{
			Name: "git",
			Args: []string{"subtree", "pull", "--prefix", project, upstreamURL, upstream.Branch},
			Dir:  Some(repoRoot),
		}
		stdout, err := subtreePullCmd.Run(ctx, cmdx.RunOptions{CaptureStdout: true})
		if err != nil {
			if stdout != "" {
				ctx.Info("subtree pull stdout", "output", stdout)
			}
			return err
		}
	}

	return nil
}

func loadWorkspaceConfig(repoRoot string) (_ config.WorkspaceConfig, retErr error) {
	path := filepath.Join(repoRoot, "meta", "repo-configuration.json")
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
