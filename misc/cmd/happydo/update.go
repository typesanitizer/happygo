package main

import (
	"fmt"

	"github.com/typesanitizer/happygo/common/cmdx"
	. "github.com/typesanitizer/happygo/common/core"
	"github.com/typesanitizer/happygo/common/logx"
)

func (ws Workspace) runUpdate(ctx logx.LogCtx, dir AbsPath, localBranch string, projects []string) error {
	for _, project := range projects {
		upstream, err := ws.Config.UpstreamForProject(localBranch, project)
		if err != nil {
			return err
		}
		upstreamURL := fmt.Sprintf("https://github.com/%s.git", upstream.GitHubRepo)
		ctx.Info("running subtree pull", "project", project, "upstream", upstreamURL, "upstream_branch", upstream.Branch)
		subtreePullCmd := cmdx.New(
			"git", "subtree", "pull", "--prefix", project, upstreamURL, upstream.Branch,
		).In(dir)
		stdout, err := ws.Runner.Run(ctx, subtreePullCmd, cmdx.RunOptionsDefault().WithCaptureStdout())
		if err != nil {
			if stdout != "" {
				ctx.Info("subtree pull stdout", "output", stdout)
			}
			return err
		}
	}
	return nil
}
