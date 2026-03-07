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

func runSyncBranch(ctx logx.LogCtx, projects []string, options RunSyncBranchOptions) (err error) {
	assert.Precondition(len(projects) > 0, "must sync 1+ projects")
	base := options.Base.ValueOr("main")

	repoRoot, err := cmdx.Cmd{
		Name: "git",
		Args: []string{"rev-parse", "--show-toplevel"},
		Dir:  None[string](),
	}.Run(ctx, cmdx.RunOptions{CaptureStdout: true})
	if err != nil {
		return errorx.Wrapf("nostack", err, "determine git repository root")
	}
	repoRoot = strings.TrimSpace(repoRoot)
	assert.Postcondition(repoRoot != "", "git rev-parse --show-toplevel returned empty output")
	fetchBaseCmd := cmdx.Cmd{
		Name: "git",
		Args: []string{"fetch", "origin", base},
		Dir:  Some(repoRoot),
	}
	if _, err := fetchBaseCmd.Run(ctx, cmdx.RunOptionsDefault()); err != nil {
		return err
	}

	worktreeDir, cleanup, err := createSyncWorktree(ctx, repoRoot, base)
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
		err = runSyncBranchProject(ctx, project, worktreeDir, base, options.Push)
		if err != nil {
			return err
		}
	}

	if !options.Push && !options.Persist {
		ctx.Info("sync run complete with no push and no persistence", "push", false, "persist", false)
	}
	return nil
}

func runSyncBranchProject(ctx logx.LogCtx, project string, worktreeDir string, mappingBranch string, push bool) error {
	syncBranch := syncBranchPrefix + project
	ctx.Info(
		"syncing project",
		"project", project,
		"branch", syncBranch,
		"worktree", worktreeDir,
		"mapping_branch", mappingBranch,
	)

	if err := resetWorktreeToBase(ctx, worktreeDir, mappingBranch); err != nil {
		return err
	}
	if err := deleteLocalBranchIfPresent(ctx, worktreeDir, syncBranch); err != nil {
		return err
	}
	if err := fetchRemoteBranchIfPresent(ctx, worktreeDir, syncBranch); err != nil {
		return err
	}

	checkoutSyncBranchCmd := cmdx.Cmd{
		Name: "git",
		Args: []string{"checkout", "-B", syncBranch},
		Dir:  Some(worktreeDir),
	}
	if _, err := checkoutSyncBranchCmd.Run(ctx, cmdx.RunOptionsDefault()); err != nil {
		return err
	}

	if err := runUpdate(ctx, worktreeDir, mappingBranch, []string{project}); err != nil {
		return err
	}

	if !push {
		ctx.Info("push disabled for sync branch", "project", project, "branch", syncBranch)
		return nil
	}

	ctx.Info("pushing sync branch", "project", project, "branch", syncBranch)
	// See SYNC(id: gha-permissions).
	pushSyncBranchCmd := cmdx.Cmd{
		Name: "git",
		Args: []string{"push", "--force-with-lease", "origin", syncBranch + ":" + syncBranch},
		Dir:  Some(worktreeDir),
	}
	if _, err := pushSyncBranchCmd.Run(ctx, cmdx.RunOptionsDefault()); err != nil {
		return err
	}
	return nil
}

func resetWorktreeToBase(ctx logx.LogCtx, worktreeDir string, base string) error {
	checkoutDetachCmd := cmdx.Cmd{
		Name: "git",
		Args: []string{"checkout", "--detach", "origin/" + base},
		Dir:  Some(worktreeDir),
	}
	if _, err := checkoutDetachCmd.Run(ctx, cmdx.RunOptionsDefault()); err != nil {
		return err
	}

	resetHardCmd := cmdx.Cmd{
		Name: "git",
		Args: []string{"reset", "--hard", "origin/" + base},
		Dir:  Some(worktreeDir),
	}
	if _, err := resetHardCmd.Run(ctx, cmdx.RunOptionsDefault()); err != nil {
		return err
	}

	cleanCmd := cmdx.Cmd{
		Name: "git",
		Args: []string{"clean", "-fd"},
		Dir:  Some(worktreeDir),
	}
	if _, err := cleanCmd.Run(ctx, cmdx.RunOptionsDefault()); err != nil {
		return err
	}
	return nil
}

func deleteLocalBranchIfPresent(ctx logx.LogCtx, worktreeDir string, branch string) error {
	listBranchCmd := cmdx.Cmd{
		Name: "git",
		Args: []string{"branch", "--list", branch},
		Dir:  Some(worktreeDir),
	}
	out, err := listBranchCmd.Run(ctx, cmdx.RunOptions{CaptureStdout: true})
	if err != nil {
		return err
	}
	if strings.TrimSpace(out) == "" {
		return nil
	}

	deleteBranchCmd := cmdx.Cmd{
		Name: "git",
		Args: []string{"branch", "-D", branch},
		Dir:  Some(worktreeDir),
	}
	_, err = deleteBranchCmd.Run(ctx, cmdx.RunOptionsDefault())
	return err
}

func fetchRemoteBranchIfPresent(ctx logx.LogCtx, worktreeDir string, branch string) error {
	lsRemoteCmd := cmdx.Cmd{
		Name: "git",
		Args: []string{"ls-remote", "--heads", "origin", branch},
		Dir:  Some(worktreeDir),
	}
	out, err := lsRemoteCmd.Run(ctx, cmdx.RunOptions{CaptureStdout: true})
	if err != nil {
		return err
	}
	if strings.TrimSpace(out) == "" {
		return nil
	}

	fetchBranchCmd := cmdx.Cmd{
		Name: "git",
		Args: []string{"fetch", "origin", branch},
		Dir:  Some(worktreeDir),
	}
	_, err = fetchBranchCmd.Run(ctx, cmdx.RunOptionsDefault())
	return err
}

func createSyncWorktree(ctx logx.LogCtx, repoRoot string, base string) (string, func() error, error) {
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
			removeWorktreeCmd := cmdx.Cmd{
				Name: "git",
				Args: []string{"worktree", "remove", "--force", worktreeDir},
				Dir:  Some(repoRoot),
			}
			if _, removeErr := removeWorktreeCmd.Run(ctx, cmdx.RunOptionsDefault()); removeErr != nil {
				cleanupErr = errorx.Join(cleanupErr, removeErr)
			}
		}
		if removeErr := os.RemoveAll(worktreeDir); removeErr != nil {
			cleanupErr = errorx.Join(cleanupErr, errorx.Wrapf("+stacks", removeErr, "remove %q", worktreeDir))
		}
		return cleanupErr
	}

	ctx.Info("adding detached sync worktree", "base", base, "worktree", worktreeDir)
	addWorktreeCmd := cmdx.Cmd{
		Name: "git",
		Args: []string{"worktree", "add", "--quiet", "--detach", worktreeDir, "origin/" + base},
		Dir:  Some(repoRoot),
	}
	if _, err := addWorktreeCmd.Run(ctx, cmdx.RunOptionsDefault()); err != nil {
		return "", nil, errorx.Join(err, cleanup())
	}
	worktreeAdded = true

	checkoutDetachCmd := cmdx.Cmd{
		Name: "git",
		Args: []string{"checkout", "--detach", "origin/" + base},
		Dir:  Some(worktreeDir),
	}
	if _, err := checkoutDetachCmd.Run(ctx, cmdx.RunOptionsDefault()); err != nil {
		return "", nil, errorx.Join(err, cleanup())
	}
	ctx.Info("worktree ready for sync", "worktree", worktreeDir, "base", base)
	return worktreeDir, cleanup, nil
}
