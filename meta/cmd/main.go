package main

import (
	"context"
	"os"
	"sort"
	"sync"
	"time"

	"golang.org/x/term"

	. "github.com/typesanitizer/happygo/common/core"
	"github.com/typesanitizer/happygo/common/errorx"
	"github.com/typesanitizer/happygo/common/logx"
	"github.com/urfave/cli/v3"
)

const syncBranchPrefix = "merge-bot/sync/"

func main() {
	logger := logx.NewLogger(os.Stderr, term.IsTerminal(int(os.Stderr.Fd())))
	getWorkspace := sync.OnceValues(newWorkspaceFromGit)
	app := &cli.Command{
		Name:  "meta",
		Usage: "Perform workspace-related administrative tasks",
		Commands: []*cli.Command{
			{
				Name: "sync-branch",
				Usage: "update and optionally push " + syncBranchPrefix +
					"<project> for --project=go|tools|delve|all using a separate worktree",
				Flags: []cli.Flag{
					&cli.StringFlag{Name: "project", Required: true},
					&cli.StringFlag{Name: "base"},
					&cli.BoolFlag{Name: "push"},
					&cli.BoolFlag{Name: "persist"},
				},
				Action: func(ctx context.Context, cmd *cli.Command) error {
					projects, err := resolveProjects(getWorkspace, cmd.String("project"))
					if err != nil {
						return err
					}
					logCtx := logx.NewLogCtx(ctx, logger)
					return runSyncBranch(logCtx, projects, RunSyncBranchOptions{
						Base:    NewOption(cmd.String("base"), cmd.IsSet("base")),
						Push:    cmd.Bool("push"),
						Persist: cmd.Bool("persist"),
					})
				},
			},
			{
				Name: "sync-pr",
				Usage: "create/update PRs for " + syncBranchPrefix +
					"<project> with labels and auto-merge",
				Flags: []cli.Flag{
					&cli.StringFlag{Name: "project", Required: true},
					&cli.StringFlag{Name: "base"},
				},
				Action: func(ctx context.Context, cmd *cli.Command) error {
					projects, err := resolveProjects(getWorkspace, cmd.String("project"))
					if err != nil {
						return err
					}
					logCtx := logx.NewLogCtx(ctx, logger)
					return runSyncPR(logCtx, projects, RunSyncPROptions{
						Base: NewOption(cmd.String("base"), cmd.IsSet("base")),
					})
				},
			},
		},
	}

	ctx, cancel := context.WithTimeoutCause(
		context.Background(), 5*time.Minute,
		errorx.Newf("nostack", "command exceeded time limit of 5 minutes"),
	)
	defer cancel()
	if err := app.Run(ctx, os.Args); err != nil {
		logger.Error(err)
		os.Exit(1)
	}
}

// resolveProjects maps "all" to the full forked folder list from workspace config,
// or validates that a single project name exists in the config.
func resolveProjects(getWorkspace func() (Workspace, error), project string) ([]string, error) {
	ws, err := getWorkspace()
	if err != nil {
		return nil, err
	}
	if project == "all" {
		var projects []string
		for folder := range ws.Config.ForkedFolders {
			projects = append(projects, folder)
		}
		sort.Strings(projects)
		return projects, nil
	}
	if _, ok := ws.Config.ForkedFolders[project]; !ok {
		return nil, errorx.Newf("nostack", "invalid --project %q, not a forked folder", project)
	}
	return []string{project}, nil
}
