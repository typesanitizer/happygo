package main

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/typesanitizer/happygo/common/assert"
	"github.com/typesanitizer/happygo/common/cmdx"
	. "github.com/typesanitizer/happygo/common/core"
	"github.com/typesanitizer/happygo/common/errorx"
	"github.com/typesanitizer/happygo/common/logx"
)

type RunSyncBranchOptions struct {
	Base    Option[string]
	Push    bool
	Persist bool
}

func (ws Workspace) runSyncBranch(ctx logx.LogCtx, projects []string, options RunSyncBranchOptions) (err error) {
	assert.Precondition(len(projects) > 0, "must sync 1+ projects")
	baseBranch := options.Base.ValueOr("main")
	fetchBaseCmd := cmdx.New("git", "fetch", "origin", baseBranch).In(ws.RepoRoot)
	if _, err := fetchBaseCmd.Run(ctx, cmdx.RunOptionsDefault()); err != nil {
		return err
	}
	worktreeDir, cleanup, err := createSyncWorktree(ctx, ws.RepoRoot, baseBranch)
	if err != nil {
		return err
	}
	defer func() {
		if options.Persist {
			ctx.Info("persisting sync worktree", "worktree", worktreeDir)
			return
		}
		err = errorx.Join(err, cleanup())
	}()
	for _, project := range projects {
		err = runSyncBranchProject(ctx, ws, project, worktreeDir, baseBranch, options.Push)
		if err != nil {
			return err
		}
	}
	if !options.Push && !options.Persist {
		ctx.Info("sync run complete", "push", false, "persist", false)
	}
	return nil
}

func runSyncBranchProject(
	ctx logx.LogCtx, ws Workspace, project string, worktreeDir string, baseBranch string, push bool,
) error {
	syncBranch := syncBranchPrefix + project
	ctx.Info(
		"syncing",
		"project", project, "branch", syncBranch,
		"worktree", worktreeDir, "base", baseBranch,
	)
	if err := resetWorktreeToBase(ctx, worktreeDir, baseBranch); err != nil {
		return errorx.Wrapf("nostack", err, "reset worktree to base %q", baseBranch)
	}
	if err := deleteLocalBranchIfPresent(ctx, worktreeDir, syncBranch); err != nil {
		return errorx.Wrapf("nostack", err, "delete local branch %q", syncBranch)
	}
	if err := fetchRemoteBranchIfPresent(ctx, worktreeDir, syncBranch); err != nil {
		return errorx.Wrapf("nostack", err, "fetch remote branch %q", syncBranch)
	}
	checkoutCmd := cmdx.New("git", "checkout", "-B", syncBranch).In(worktreeDir)
	if _, err := checkoutCmd.Run(ctx, cmdx.RunOptionsDefault()); err != nil {
		return err
	}
	if err := ws.runUpdate(ctx, worktreeDir, baseBranch, []string{project}); err != nil {
		return err
	}
	if !push {
		ctx.Info("skipping push", "project", project, "branch", syncBranch)
		return nil
	}

	ctx.Info("pushing sync branch", "project", project, "branch", syncBranch)
	// See SYNC(id: gha-permissions).
	pushCmd := cmdx.New("git", "push", "--force-with-lease", "origin", syncBranch+":"+syncBranch).
		In(worktreeDir)
	if _, err := pushCmd.Run(ctx, cmdx.RunOptionsDefault()); err != nil {
		return err
	}
	return nil
}

func resetWorktreeToBase(ctx logx.LogCtx, worktreeDir string, base string) error {
	return cmdx.ExecAll(ctx,
		cmdx.New("git", "checkout", "--detach", "origin/"+base).In(worktreeDir),
		cmdx.New("git", "reset", "--hard", "origin/"+base).In(worktreeDir),
		cmdx.New("git", "clean", "-fd").In(worktreeDir),
	)
}

func deleteLocalBranchIfPresent(ctx logx.LogCtx, worktreeDir string, branch string) error {
	listCmd := cmdx.New("git", "branch", "--list", branch).In(worktreeDir)
	out, err := listCmd.Run(ctx, cmdx.RunOptions{CaptureStdout: true})
	if err != nil {
		return err
	}
	if strings.TrimSpace(out) == "" {
		return nil
	}
	deleteCmd := cmdx.New("git", "branch", "-D", branch).In(worktreeDir)
	_, err = deleteCmd.Run(ctx, cmdx.RunOptionsDefault())
	return err
}

func fetchRemoteBranchIfPresent(ctx logx.LogCtx, worktreeDir string, branch string) error {
	lsRemoteCmd := cmdx.New("git", "ls-remote", "--heads", "origin", branch).In(worktreeDir)
	out, err := lsRemoteCmd.Run(ctx, cmdx.RunOptions{CaptureStdout: true})
	if err != nil {
		return err
	}
	if strings.TrimSpace(out) == "" {
		return nil
	}
	fetchCmd := cmdx.New("git", "fetch", "origin", branch).In(worktreeDir)
	_, err = fetchCmd.Run(ctx, cmdx.RunOptionsDefault())
	return err
}

func createSyncWorktree(
	ctx logx.LogCtx, repoRoot string, base string,
) (string, func() error, error) {
	tmpRoot := filepath.Join(repoRoot, ".cache", "tmp")
	if err := os.MkdirAll(tmpRoot, 0o755); err != nil {
		return "", nil, errorx.Wrapf("+stacks", err, "create temp root %q", tmpRoot)
	}
	worktreeDir, err := os.MkdirTemp(tmpRoot, "meta-sync-")
	if err != nil {
		return "", nil, errorx.Wrapf("+stacks", err, "create sync worktree")
	}

	worktreeAdded := false
	cleanup := func() error {
		var cleanupErr error
		if worktreeAdded {
			removeCmd := cmdx.New("git", "worktree", "remove", "--force", worktreeDir).
				In(repoRoot)
			if _, removeErr := removeCmd.Run(ctx, cmdx.RunOptionsDefault()); removeErr != nil {
				cleanupErr = errorx.Join(cleanupErr, removeErr)
			}
		}
		if removeErr := os.RemoveAll(worktreeDir); removeErr != nil {
			cleanupErr = errorx.Join(cleanupErr,
				errorx.Wrapf("+stacks", removeErr, "remove %q", worktreeDir))
		}
		return cleanupErr
	}

	ctx.Info("adding detached sync worktree", "base", base, "worktree", worktreeDir)
	addCmd := cmdx.New("git", "worktree", "add", "--quiet", "--detach", worktreeDir, "origin/"+base).
		In(repoRoot)
	if _, err := addCmd.Run(ctx, cmdx.RunOptionsDefault()); err != nil {
		return "", nil, errorx.Join(err, cleanup())
	}
	worktreeAdded = true

	detachCmd := cmdx.New("git", "checkout", "--detach", "origin/"+base).In(worktreeDir)
	if _, err := detachCmd.Run(ctx, cmdx.RunOptionsDefault()); err != nil {
		return "", nil, errorx.Join(err, cleanup())
	}
	ctx.Info("worktree ready for sync", "worktree", worktreeDir, "base", base)
	return worktreeDir, cleanup, nil
}
