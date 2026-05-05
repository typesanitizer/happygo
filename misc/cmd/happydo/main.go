// Copyright 2026 Varun Gandhi
//
// SPDX-License-Identifier: PolyForm-Noncommercial-1.0.0

package main

import (
	"context"
	"io"
	"os"
	"strings"
	"sync"

	"github.com/urfave/cli/v3"

	"github.com/typesanitizer/happygo/common/cmdx"
	"github.com/typesanitizer/happygo/common/collections"
	. "github.com/typesanitizer/happygo/common/core"
	"github.com/typesanitizer/happygo/common/errorx"
	"github.com/typesanitizer/happygo/common/fsx"
	"github.com/typesanitizer/happygo/common/fsx/fsx_name"
	"github.com/typesanitizer/happygo/common/logx"
	"github.com/typesanitizer/happygo/common/syscaps"
	"github.com/typesanitizer/happygo/common/time"
	"github.com/typesanitizer/happygo/misc/internal/config"
)

const syncBranchPrefix = "merge-bot/sync/"

// Workspace provides operations over the repository root using the repo configuration.
type Workspace struct {
	FS     fsx.FS
	Config config.WorkspaceConfig
	Runner cmdx.Runner
}

func newWorkspaceFromGit(runner cmdx.Runner) (Workspace, error) {
	repoRootCmd := cmdx.New("git", "rev-parse", "--show-toplevel")
	ctx := logx.NewLogCtx(context.Background(), logx.NewLogger(io.Discard, logx.ColorSupport_Disable))
	out, err := runner.Run(ctx, repoRootCmd, cmdx.RunOptionsDefault().WithCaptureStdout())
	if err != nil {
		return Workspace{}, errorx.Wrapf("nostack", err, "determine git repository root")
	}
	repoRoot := NewAbsPath(strings.TrimSpace(out))
	repoFS, err := syscaps.FS(repoRoot)
	if err != nil {
		return Workspace{}, errorx.Wrapf("+stacks", err, "open repo filesystem at %s", repoRoot)
	}
	wsConfig, err := loadWorkspaceConfig(repoFS)
	return Workspace{FS: repoFS, Config: wsConfig, Runner: runner}, err
}

func main() {
	logger := logx.NewLogger(os.Stderr, logx.ColorSupport_AutoDetect)
	runner := syscaps.CmdRunner{Env: syscaps.Env()}
	getWorkspace := sync.OnceValues(func() (Workspace, error) {
		return newWorkspaceFromGit(runner)
	})
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
					projectArg, err := fsx_name.Parse(cmd.String("project"))
					if err != nil {
						return errorx.Wrapf("nostack", err, "in argument for --project")
					}

					ctx, cancel := withTimeout(ctx, 5*time.Minute, cmd.Name)
					defer cancel()
					ws, projects, err := resolveProjects(getWorkspace, projectArg)
					if err != nil {
						return err
					}
					logCtx := logx.NewLogCtx(ctx, logger)
					return ws.runSyncBranch(logCtx, projects, RunSyncBranchOptions{
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
					projectArg, err := fsx_name.Parse(cmd.String("project"))
					if err != nil {
						return errorx.Wrapf("nostack", err, "in argument for --project")
					}

					ctx, cancel := withTimeout(ctx, 5*time.Minute, cmd.Name)
					defer cancel()
					ws, projects, err := resolveProjects(getWorkspace, projectArg)
					if err != nil {
						return err
					}
					logCtx := logx.NewLogCtx(ctx, logger)
					clock := syscaps.SystemClock()
					return ws.runSyncPR(logCtx, clock, projects, RunSyncPROptions{
						Base: NewOption(cmd.String("base"), cmd.IsSet("base")),
					})
				},
			},
			{
				Name:  "list",
				Usage: "list workspace items",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name: "type", Required: true,
						Usage: "item type: go-module",
					},
					&cli.StringFlag{
						Name:  "provenance",
						Usage: "provenance filter: first-party, forked (default: all)",
					},
				},
				Action: func(_ context.Context, cmd *cli.Command) error {
					ws, err := getWorkspace()
					if err != nil {
						return err
					}
					var type_ ListType
					switch cmd.String("type") {
					case "go-module":
						type_ = ListType_GoModules
					default:
						return errorx.Newf("nostack",
							"unknown --type %q, want go-module", cmd.String("type"))
					}
					provenance := ListProvenance_All
					if cmd.IsSet("provenance") {
						switch cmd.String("provenance") {
						case "first-party":
							provenance = ListProvenance_FirstParty
						case "forked":
							provenance = ListProvenance_Forked
						default:
							return errorx.Newf("nostack",
								"unknown --provenance %q, want first-party|forked", cmd.String("provenance"))
						}
					}
					return ws.List(logger, os.Stdout, ListOptions{Type: type_, Provenance: provenance})
				},
			},
		},
	}

	if err := app.Run(context.Background(), os.Args); err != nil {
		logger.Error(err)
		os.Exit(1)
	}
}

// resolveProjects maps "all" to the full forked folder list from workspace config,
// or validates that a single project name exists in the config.
func resolveProjects(getWorkspace func() (Workspace, error), project fsx.Name) (Workspace, []fsx.Name, error) {
	ws, err := getWorkspace()
	if err != nil {
		return Workspace{}, nil, err
	}
	if project.String() == "all" {
		return ws, collections.SortedMapKeysFunc(ws.Config.ForkedFolders, fsx.Name.Compare), nil
	}
	if _, ok := ws.Config.ForkedFolders[project]; !ok {
		return Workspace{}, nil, errorx.Newf("nostack", "invalid --project %q, not a forked folder", project)
	}
	return ws, []fsx.Name{project}, nil
}

func withTimeout(ctx context.Context, duration time.Duration, cmdName string) (context.Context, context.CancelFunc) {
	return context.WithTimeoutCause(
		ctx, duration,
		errorx.Newf("nostack", "%s exceeded time limit of %s", cmdName, duration),
	)
}
