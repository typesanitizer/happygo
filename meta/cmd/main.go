package main

import (
	"context"
	"os"
	"sync"
	"time"

	"github.com/urfave/cli/v3"

	"github.com/typesanitizer/happygo/common/collections"
	. "github.com/typesanitizer/happygo/common/core"
	"github.com/typesanitizer/happygo/common/errorx"
	"github.com/typesanitizer/happygo/common/logx"
)

const syncBranchPrefix = "merge-bot/sync/"

func main() {
	logger := logx.NewLogger(os.Stderr, logx.ColorSupport_AutoDetect)
	getWorkspace := sync.OnceValues(newWorkspaceFromGit)
	app := &cli.Command{
		Name:  "meta",
		Usage: "Perform workspace-related administrative tasks",
		Commands: append([]*cli.Command{
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
					ws, projects, err := resolveProjects(getWorkspace, cmd.String("project"))
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
					ws, projects, err := resolveProjects(getWorkspace, cmd.String("project"))
					if err != nil {
						return err
					}
					logCtx := logx.NewLogCtx(ctx, logger)
					return ws.runSyncPR(logCtx, projects, RunSyncPROptions{
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
		}, newAgentLoopCommands(logger, getWorkspace)...),
	}

	timeout := 5 * time.Minute
	timeoutMessage := "command exceeded time limit of 5 minutes"
	if len(os.Args) > 1 && (os.Args[1] == "agent-loop" || os.Args[1] == "agent-loop-inner") {
		timeout = 24 * time.Hour
		timeoutMessage = "command exceeded time limit of 24 hours"
	}
	ctx, cancel := context.WithTimeoutCause(
		context.Background(), timeout,
		errorx.Newf("nostack", "%s", timeoutMessage),
	)
	defer cancel()
	if err := app.Run(ctx, os.Args); err != nil {
		logger.Error(err)
		os.Exit(1)
	}
}

// resolveProjects maps "all" to the full forked folder list from workspace config,
// or validates that a single project name exists in the config.
func resolveProjects(getWorkspace func() (Workspace, error), project string) (Workspace, []string, error) {
	ws, err := getWorkspace()
	if err != nil {
		return Workspace{}, nil, err
	}
	if project == "all" {
		return ws, collections.SortedMapKeys(ws.Config.ForkedFolders), nil
	}
	if _, ok := ws.Config.ForkedFolders[project]; !ok {
		return Workspace{}, nil, errorx.Newf("nostack", "invalid --project %q, not a forked folder", project)
	}
	return ws, []string{project}, nil
}
