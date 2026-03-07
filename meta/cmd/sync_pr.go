package main

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/typesanitizer/happygo/common/assert"
	"github.com/typesanitizer/happygo/common/cmdx"
	. "github.com/typesanitizer/happygo/common/core"
	"github.com/typesanitizer/happygo/common/errorx"
	"github.com/typesanitizer/happygo/common/logx"
)

type RunSyncPROptions struct {
	Base Option[string]
}

type pullRequest struct {
	Number int    `json:"number"`
	URL    string `json:"url"`
}

const (
	mergebotSubtreeDirTrailer     = "mergebot-subtree-dir"
	mergebotLocalCommitTrailer    = "mergebot-local-commit"
	mergebotUpstreamCommitTrailer = "mergebot-upstream-commit"
)

// NOTE(id: sync-pr-subtree-parents): For sync branches produced by sync-branch,
// the branch head must be a 2-parent merge commit from `git subtree pull`.
// Parent 1 is the local pre-sync commit, and parent 2 is the upstream subtree
// commit that was pulled. We persist these values in mergebot-* trailers.
type parsedSubtreeMetadata struct {
	Dir            string
	LocalCommit    string
	UpstreamCommit string
}

// subtreeMetadata is a validated parsedSubtreeMetadata with all fields guaranteed non-empty.
type subtreeMetadata parsedSubtreeMetadata

func (p parsedSubtreeMetadata) validate() (subtreeMetadata, error) {
	if p.Dir == "" || p.LocalCommit == "" || p.UpstreamCommit == "" {
		return subtreeMetadata{}, errorx.Newf("nostack",
			"incomplete subtree metadata: dir=%q local=%q upstream=%q",
			p.Dir, p.LocalCommit, p.UpstreamCommit)
	}
	return subtreeMetadata(p), nil
}

func runSyncPR(ctx logx.LogCtx, projects []string, options RunSyncPROptions) error {
	assert.Precondition(len(projects) > 0, "must sync 1+ projects")
	base := options.Base.ValueOr("main")

	repoRootCmd := cmdx.Cmd{
		Name: "git",
		Args: []string{"rev-parse", "--show-toplevel"},
		Dir:  None[string](),
	}
	repoRoot, err := repoRootCmd.Run(ctx, cmdx.RunOptions{CaptureStdout: true})
	if err != nil {
		return errorx.Wrapf("nostack", err, "determine git repository root")
	}
	repoRoot = strings.TrimSpace(repoRoot)
	assert.Postcondition(repoRoot != "", "git rev-parse --show-toplevel returned empty output")

	for _, project := range projects {
		if err := runSyncPRProject(ctx, repoRoot, project, base); err != nil {
			return err
		}
	}
	return nil
}

func runSyncPRProject(ctx logx.LogCtx, repoRoot string, project string, base string) error {
	assert.Precondition(repoRoot != "", "repoRoot must be non-empty")
	repoRootDir := Some(repoRoot)

	syncBranch := syncBranchPrefix + project
	fetchCmd := cmdx.Cmd{
		Name: "git",
		Args: []string{"fetch", "origin", syncBranch},
		Dir:  repoRootDir,
	}
	if _, err := fetchCmd.Run(ctx, cmdx.RunOptionsDefault()); err != nil {
		return err
	}

	// The push in sync-branch is a no-op when upstream hasn't changed,
	// but sync-pr still runs unconditionally. Skip PR creation/updates
	// when the sync branch has no diff vs base to avoid noisy re-edits.
	diffCmd := cmdx.Cmd{
		Name: "git",
		Args: []string{"diff", "--quiet", "origin/" + base + "...origin/" + syncBranch},
		Dir:  repoRootDir,
	}
	if _, err := diffCmd.Run(ctx, cmdx.RunOptionsDefault()); err == nil {
		ctx.Info("no diff between base and sync branch, skipping", "project", project, "base", base, "branch", syncBranch)
		return nil
	}

	headSHACmd := cmdx.Cmd{
		Name: "git",
		Args: []string{"rev-parse", "origin/" + syncBranch},
		Dir:  repoRootDir,
	}
	headSHA, err := headSHACmd.Run(ctx, cmdx.RunOptions{CaptureStdout: true})
	if err != nil {
		return err
	}
	headSHA = strings.TrimSpace(headSHA)
	metadata, err := subtreeMetadataForSyncHead(ctx, repoRoot, project, headSHA)
	if err != nil {
		return err
	}

	title := fmt.Sprintf("chore(%s): Sync changes from upstream (%s)", project, time.Now().UTC().Format("2006-01-02"))
	body := syncPRBody(metadata.UpstreamCommit)

	prs, err := listOpenSyncPRs(ctx, repoRoot, base, syncBranch)
	if err != nil {
		return err
	}

	projectLabel := "project/" + project
	ensureSyncLabels := func() error {
		if err := ensureLabelExists(ctx, repoRoot, projectLabel, "1d76db", "Project-specific sync updates"); err != nil {
			return err
		}
		if err := ensureLabelExists(ctx, repoRoot, "upstream-sync", "6e7781", "Automated upstream sync updates"); err != nil {
			return err
		}
		return nil
	}

	var prNumber int
	switch len(prs) {
	case 0:
		if err := ensureSyncLabels(); err != nil {
			return err
		}
		ctx.Info("creating sync PR", "project", project, "branch", syncBranch, "base", base)
		// See SYNC(id: gha-permissions).
		createPRCmd := cmdx.Cmd{
			Name: "gh",
			Args: []string{"pr", "create", "--base", base, "--head", syncBranch, "--title", title, "--body", body, "--label", projectLabel, "--label", "upstream-sync"},
			Dir:  repoRootDir,
		}
		if _, err := createPRCmd.Run(ctx, cmdx.RunOptionsDefault()); err != nil {
			return err
		}
		prs, err = listOpenSyncPRs(ctx, repoRoot, base, syncBranch)
		if err != nil {
			return err
		}
		if len(prs) != 1 {
			return errorx.Newf("nostack", "expected 1 open PR for branch %q after creation, got %d", syncBranch, len(prs))
		}
		prNumber = prs[0].Number
	case 1:
		if err := ensureSyncLabels(); err != nil {
			return err
		}
		prNumber = prs[0].Number
		prRef := strconv.Itoa(prNumber)
		// See SYNC(id: gha-permissions).
		editPRCmd := cmdx.Cmd{
			Name: "gh",
			Args: []string{"pr", "edit", prRef, "--title", title, "--body", body, "--add-label", projectLabel, "--add-label", "upstream-sync"},
			Dir:  repoRootDir,
		}
		if _, err := editPRCmd.Run(ctx, cmdx.RunOptionsDefault()); err != nil {
			return err
		}
	default:
		return errorx.Newf("nostack", "found %d open PRs for branch %q into %q", len(prs), syncBranch, base)
	}

	prRef := strconv.Itoa(prNumber)

	mergeBody := syncMergeBody(metadata)
	// See SYNC(id: gha-permissions).
	mergePRCmd := cmdx.Cmd{
		Name: "gh",
		Args: []string{"pr", "merge", prRef, "--auto", "--merge", "--subject", title, "--body", mergeBody, "--match-head-commit", headSHA},
		Dir:  repoRootDir,
	}
	if _, err := mergePRCmd.Run(ctx, cmdx.RunOptionsDefault()); err != nil {
		return err
	}
	return nil
}

func subtreeMetadataForSyncHead(ctx logx.LogCtx, repoRoot string, project string, headSHA string) (subtreeMetadata, error) {
	assert.Precondition(repoRoot != "", "repoRoot must be non-empty")
	assert.Precondition(project != "", "project must be non-empty")
	assert.Precondition(headSHA != "", "headSHA must be non-empty")

	var emptyMetadata subtreeMetadata

	parentsCmd := cmdx.Cmd{
		Name: "git",
		Args: []string{"show", "-s", "--format=%P", headSHA},
		Dir:  Some(repoRoot),
	}
	parentsOut, err := parentsCmd.Run(ctx, cmdx.RunOptions{CaptureStdout: true})
	if err != nil {
		return emptyMetadata, err
	}

	parentSHAs := strings.Fields(strings.TrimSpace(parentsOut))
	if len(parentSHAs) != 2 {
		return emptyMetadata, errorx.Newf("nostack", "expected sync head %q for %q to be a 2-parent merge commit, got %d parent(s)", headSHA, project, len(parentSHAs))
	}

	return (parsedSubtreeMetadata{
		Dir:            project,
		LocalCommit:    parentSHAs[0],
		UpstreamCommit: parentSHAs[1],
	}).validate()
}

func listOpenSyncPRs(ctx logx.LogCtx, repoRoot string, base string, head string) ([]pullRequest, error) {
	assert.Precondition(repoRoot != "", "repoRoot must be non-empty")
	repoRootDir := Some(repoRoot)

	listPRsCmd := cmdx.Cmd{
		Name: "gh",
		Args: []string{"pr", "list", "--state", "open", "--base", base, "--head", head, "--json", "number,url"},
		Dir:  repoRootDir,
	}
	out, err := listPRsCmd.Run(ctx, cmdx.RunOptions{CaptureStdout: true})
	if err != nil {
		return nil, err
	}
	var prs []pullRequest
	if err := json.Unmarshal([]byte(out), &prs); err != nil {
		return nil, errorx.Wrapf("+stacks", err, "parse gh pr list output: %s", out)
	}
	return prs, nil
}

func ensureLabelExists(ctx logx.LogCtx, repoRoot string, name string, color string, description string) error {
	assert.Precondition(repoRoot != "", "repoRoot must be non-empty")
	repoRootDir := Some(repoRoot)

	listLabelsCmd := cmdx.Cmd{
		Name: "gh",
		Args: []string{"label", "list", "--search", name, "--json", "name", "--limit", "100"},
		Dir:  repoRootDir,
	}
	out, err := listLabelsCmd.Run(ctx, cmdx.RunOptions{CaptureStdout: true})
	if err != nil {
		return err
	}
	var labels []struct {
		Name string `json:"name"`
	}
	// gh label list --search returns empty output (not "[]") when no labels match.
	if out = strings.TrimSpace(out); out != "" {
		if err := json.Unmarshal([]byte(out), &labels); err != nil {
			return errorx.Wrapf("+stacks", err, "parse gh label list output: %s", out)
		}
	}
	for _, label := range labels {
		if label.Name == name {
			return nil
		}
	}
	// See SYNC(id: gha-permissions).
	createLabelCmd := cmdx.Cmd{
		Name: "gh",
		Args: []string{"label", "create", name, "--color", color, "--description", description},
		Dir:  repoRootDir,
	}
	_, err = createLabelCmd.Run(ctx, cmdx.RunOptionsDefault())
	return err
}

func syncPRBody(upstreamCommit string) string {
	return fmt.Sprintf("Pull in changes from upstream commit %s", upstreamCommit)
}

func syncMergeBody(metadata subtreeMetadata) string {
	lines := []string{
		fmt.Sprintf("Pull in changes from upstream commit %s", metadata.UpstreamCommit),
		"",
		mergebotSubtreeDirTrailer + ": " + metadata.Dir,
		mergebotLocalCommitTrailer + ": " + metadata.LocalCommit,
		mergebotUpstreamCommitTrailer + ": " + metadata.UpstreamCommit,
	}
	return strings.Join(lines, "\n")
}
