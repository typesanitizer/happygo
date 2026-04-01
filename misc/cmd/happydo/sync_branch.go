package main

import (
	"os"
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

type remoteRef struct {
	Name string
	SHA  string
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
			ctx.Info("persisting sync worktree", "worktree", worktreeDir.String())
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
	ctx logx.LogCtx, ws Workspace, project string, worktreeDir AbsPath, baseBranch string, push bool,
) error {
	syncBranch := syncBranchPrefix + project
	ctx.Info(
		"syncing",
		"project", project, "branch", syncBranch,
		"worktree", worktreeDir.String(), "base", baseBranch,
	)
	if err := resetWorktreeToBase(ctx, worktreeDir, baseBranch); err != nil {
		return errorx.Wrapf("nostack", err, "reset worktree to base %q", baseBranch)
	}
	if err := deleteLocalBranchIfPresent(ctx, worktreeDir, syncBranch); err != nil {
		return errorx.Wrapf("nostack", err, "delete local branch %q", syncBranch)
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

	remoteHead, err := findRemoteBranchHeadRef(ctx, worktreeDir, syncBranch)
	if err != nil {
		return errorx.Wrapf("nostack", err, "find remote head ref for %q", syncBranch)
	}
	forceWithLeaseArg := "--force-with-lease=" + formatLease(syncBranch, remoteHead)
	ctx.Info("pushing sync branch", "project", project, "branch", syncBranch)
	// See SYNC(id: gha-permissions).
	pushCmd := cmdx.New("git", "push", forceWithLeaseArg, "origin", syncBranch+":"+syncBranch).
		In(worktreeDir)
	if _, err := pushCmd.Run(ctx, cmdx.RunOptionsDefault()); err != nil {
		return err
	}
	return nil
}

func resetWorktreeToBase(ctx logx.LogCtx, worktreeDir AbsPath, base string) error {
	return cmdx.ExecAll(ctx,
		cmdx.New("git", "checkout", "--detach", "origin/"+base).In(worktreeDir),
		cmdx.New("git", "reset", "--hard", "origin/"+base).In(worktreeDir),
		cmdx.New("git", "clean", "-fd").In(worktreeDir),
	)
}

func deleteLocalBranchIfPresent(ctx logx.LogCtx, worktreeDir AbsPath, branch string) error {
	listCmd := cmdx.New("git", "branch", "--list", branch).In(worktreeDir)
	out, err := listCmd.Run(ctx, cmdx.RunOptionsDefault().WithCaptureStdout())
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

func findRemoteBranchHeadRef(
	ctx logx.LogCtx, worktreeDir AbsPath, branch string,
) (Option[remoteRef], error) {
	assert.Precondition(branch != "", "branch must be non-empty")

	branchRef := "refs/heads/" + branch
	lsRemoteCmd := cmdx.New("git", "ls-remote", "--heads", "origin", branchRef).In(worktreeDir)
	out, err := lsRemoteCmd.Run(ctx, cmdx.RunOptionsDefault().WithCaptureStdout())
	if err != nil {
		return None[remoteRef](), err
	}
	return parseSingleRemoteRef(branchRef, out)
}

func parseSingleRemoteRef(wantRef string, lsRemoteOutput string) (Option[remoteRef], error) {
	assert.Precondition(wantRef != "", "wantRef must be non-empty")
	out := strings.TrimSpace(lsRemoteOutput)
	if out == "" {
		return None[remoteRef](), nil
	}
	lines := strings.Split(out, "\n")
	if len(lines) != 1 {
		return None[remoteRef](), errorx.Newf("nostack",
			"expected at most 1 ls-remote line for %q, got %d", wantRef, len(lines))
	}
	fields := strings.Fields(lines[0])
	if len(fields) != 2 {
		return None[remoteRef](), errorx.Newf("nostack",
			"expected 2 fields in ls-remote output for %q, got %d: %q",
			wantRef, len(fields), lines[0])
	}
	if fields[1] != wantRef {
		return None[remoteRef](), errorx.Newf("nostack",
			"expected ls-remote ref %q, got %q", wantRef, fields[1])
	}
	return Some(remoteRef{Name: fields[1], SHA: fields[0]}), nil
}

func formatLease(branch string, remoteHead Option[remoteRef]) string {
	assert.Precondition(branch != "", "branch must be non-empty")
	// Use the explicit <ref>:<expect> forms documented at:
	// https://git-scm.com/docs/git-push#Documentation/git-push.txt---force-with-lease
	if remoteRef, ok := remoteHead.Get(); ok {
		return branch + ":" + remoteRef.SHA
	}
	return branch + ":"
}

func createSyncWorktree(
	ctx logx.LogCtx, repoRoot AbsPath, base string,
) (AbsPath, func() error, error) {
	tmpRoot := repoRoot.JoinComponents(".cache", "tmp")
	if err := tmpRoot.MkdirAll(0o755); err != nil {
		return AbsPath{}, nil, errorx.Wrapf("+stacks", err, "create temp root %q", tmpRoot)
	}
	worktreeDir, err := tmpRoot.MkdirTemp("meta-sync-")
	if err != nil {
		return AbsPath{}, nil, errorx.Wrapf("+stacks", err, "create sync worktree")
	}

	worktreeAdded := false
	cleanup := func() error {
		var cleanupErr error
		if worktreeAdded {
			removeCmd := cmdx.New("git", "worktree", "remove", "--force", worktreeDir.String()).
				In(repoRoot)
			if _, removeErr := removeCmd.Run(ctx, cmdx.RunOptionsDefault()); removeErr != nil {
				cleanupErr = errorx.Join(cleanupErr, removeErr)
			}
		}
		if removeErr := os.RemoveAll(worktreeDir.String()); removeErr != nil {
			cleanupErr = errorx.Join(cleanupErr,
				errorx.Wrapf("+stacks", removeErr, "remove %q", worktreeDir.String()))
		}
		return cleanupErr
	}

	ctx.Info("adding detached sync worktree", "base", base, "worktree", worktreeDir.String())
	addCmd := cmdx.New("git", "worktree", "add", "--quiet", "--detach", worktreeDir.String(), "origin/"+base).
		In(repoRoot)
	if _, err := addCmd.Run(ctx, cmdx.RunOptionsDefault()); err != nil {
		return AbsPath{}, nil, errorx.Join(err, cleanup())
	}
	worktreeAdded = true

	detachCmd := cmdx.New("git", "checkout", "--detach", "origin/"+base).In(worktreeDir)
	if _, err := detachCmd.Run(ctx, cmdx.RunOptionsDefault()); err != nil {
		return AbsPath{}, nil, errorx.Join(err, cleanup())
	}
	ctx.Info("worktree ready for sync", "worktree", worktreeDir.String(), "base", base)
	return worktreeDir, cleanup, nil
}
