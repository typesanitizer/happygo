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

type ListOpenPRsData struct {
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
	Dir            Option[string]
	LocalCommit    Option[string]
	UpstreamCommit Option[string]
}

// subtreeMetadata is a validated parsedSubtreeMetadata with all fields guaranteed non-empty.
type subtreeMetadata struct {
	Dir            string
	LocalCommit    string
	UpstreamCommit string
}

func (p parsedSubtreeMetadata) validate() (subtreeMetadata, error) {
	dir, dirOk := p.Dir.Get()
	local, localOk := p.LocalCommit.Get()
	upstream, upstreamOk := p.UpstreamCommit.Get()
	if !dirOk || !localOk || !upstreamOk {
		return subtreeMetadata{}, errorx.Newf("nostack",
			"incomplete subtree metadata: dir=%v local=%v upstream=%v",
			p.Dir, p.LocalCommit, p.UpstreamCommit)
	}
	return subtreeMetadata{Dir: dir, LocalCommit: local, UpstreamCommit: upstream}, nil
}

func (ws Workspace) runSyncPR(ctx logx.LogCtx, projects []string, options RunSyncPROptions) error {
	assert.Precondition(len(projects) > 0, "must sync 1+ projects")
	base := options.Base.ValueOr("main")
	for _, project := range projects {
		if err := runSyncPRProject(ctx, ws.RepoRoot, project, base); err != nil {
			return err
		}
	}
	return nil
}

func runSyncPRProject(ctx logx.LogCtx, repoRoot string, project string, base string) error {
	assert.Precondition(repoRoot != "", "repoRoot must be non-empty")

	syncBranch := syncBranchPrefix + project
	fetchCmd := cmdx.New("git", "fetch", "origin", syncBranch).In(repoRoot)
	if _, err := fetchCmd.Run(ctx, cmdx.RunOptionsDefault()); err != nil {
		return err
	}
	// The push in sync-branch is a no-op when upstream hasn't changed,
	// but sync-pr still runs unconditionally. Skip PR creation/updates
	// when the sync branch has no diff vs base to avoid noisy re-edits.
	diffCmd := cmdx.New("git", "diff", "--quiet", "origin/"+base+"...origin/"+syncBranch).In(repoRoot)
	if _, err := diffCmd.Run(ctx, cmdx.RunOptionsDefault()); err == nil {
		ctx.Info("no diff between base and sync branch, skipping",
			"project", project, "base", base, "branch", syncBranch)
		return nil
	}
	headSHA, err := cmdx.New("git", "rev-parse", "origin/"+syncBranch).In(repoRoot).
		Run(ctx, cmdx.RunOptions{CaptureStdout: true})
	if err != nil {
		return err
	}
	headSHA = strings.TrimSpace(headSHA)
	metadata, err := subtreeMetadataForSyncHead(ctx, repoRoot, project, headSHA)
	if err != nil {
		return err
	}
	existingPR, err := findOpenPR(ctx, repoRoot, base, syncBranch)
	if err != nil {
		return err
	}
	projectLabel := "project/" + project
	ensureSyncLabels := func() error {
		if err := ensureLabelExists(ctx, repoRoot, projectLabel,
			"1d76db", "Project-specific sync updates"); err != nil {
			return err
		}
		if err := ensureLabelExists(ctx, repoRoot, "upstream-sync",
			"6e7781", "Automated upstream sync updates"); err != nil {
			return err
		}
		return nil
	}
	if err := ensureSyncLabels(); err != nil {
		return err
	}
	title := fmt.Sprintf("chore(%s): Sync changes from upstream (%s)",
		project, time.Now().UTC().Format("2006-01-02"))
	body := formatPRBody(metadata.UpstreamCommit)
	var prNumber int
	if existing, ok := existingPR.Get(); ok {
		prNumber = existing
		// See SYNC(id: gha-permissions).
		editPRCmd := cmdx.New(
			"gh", "pr", "edit", strconv.Itoa(prNumber),
			"--title", title, "--body", body,
			"--add-label", projectLabel, "--add-label", "upstream-sync",
		).In(repoRoot)
		if _, err := editPRCmd.Run(ctx, cmdx.RunOptionsDefault()); err != nil {
			return err
		}
	} else {
		ctx.Info("creating sync PR", "project", project, "branch", syncBranch, "base", base)
		// See SYNC(id: gha-permissions).
		createPRCmd := cmdx.New(
			"gh", "pr", "create",
			"--base", base, "--head", syncBranch,
			"--title", title, "--body", body,
			"--label", projectLabel, "--label", "upstream-sync",
		).In(repoRoot)
		if _, err := createPRCmd.Run(ctx, cmdx.RunOptionsDefault()); err != nil {
			return err
		}
		created, err := findOpenPR(ctx, repoRoot, base, syncBranch)
		if err != nil {
			return err
		}
		newPR, ok := created.Get()
		if !ok {
			return errorx.Newf("nostack", "expected 1 open PR for branch %q after creation, got 0",
				syncBranch)
		}
		prNumber = newPR
	}
	mergeBody := formatMergeBody(metadata)
	// See SYNC(id: gha-permissions).
	mergePRCmd := cmdx.New(
		"gh", "pr", "merge", strconv.Itoa(prNumber),
		"--auto", "--merge",
		"--subject", title, "--body", mergeBody,
		"--match-head-commit", headSHA,
	).In(repoRoot)
	if _, err := mergePRCmd.Run(ctx, cmdx.RunOptionsDefault()); err != nil {
		return err
	}
	return nil
}

func findOpenPR(
	ctx logx.LogCtx, repoRoot string, base string, head string,
) (Option[int], error) {
	prs, err := listOpenPRs(ctx, repoRoot, base, head)
	if err != nil {
		return None[int](), err
	}
	switch len(prs) {
	case 0:
		return None[int](), nil
	case 1:
		return Some(prs[0].Number), nil
	default:
		return None[int](), errorx.Newf("nostack",
			"found %d open PRs for branch %q into %q", len(prs), head, base)
	}
}

func subtreeMetadataForSyncHead(
	ctx logx.LogCtx, repoRoot string, project string, headSHA string,
) (subtreeMetadata, error) {
	assert.Precondition(repoRoot != "", "repoRoot must be non-empty")
	assert.Precondition(project != "", "project must be non-empty")
	assert.Precondition(headSHA != "", "headSHA must be non-empty")

	var emptyMetadata subtreeMetadata
	parentsOut, err := cmdx.New("git", "show", "--no-patch", "--format=%P", headSHA).In(repoRoot).
		Run(ctx, cmdx.RunOptions{CaptureStdout: true})
	if err != nil {
		return emptyMetadata, err
	}
	parentSHAs := strings.Fields(strings.TrimSpace(parentsOut))
	if len(parentSHAs) != 2 {
		return emptyMetadata, errorx.Newf("nostack",
			"expected sync head %q for %q to be a 2-parent merge commit, got %d parent(s)",
			headSHA, project, len(parentSHAs))
	}
	return (parsedSubtreeMetadata{
		Dir:            Some(project),
		LocalCommit:    Some(parentSHAs[0]),
		UpstreamCommit: Some(parentSHAs[1]),
	}).validate()
}

func listOpenPRs(
	ctx logx.LogCtx, repoRoot string, base string, head string,
) ([]ListOpenPRsData, error) {
	assert.Precondition(repoRoot != "", "repoRoot must be non-empty")

	listPRsCmd := cmdx.New(
		"gh", "pr", "list",
		"--state", "open", "--base", base, "--head", head,
		"--json", "number,url",
	).In(repoRoot)
	out, err := listPRsCmd.Run(ctx, cmdx.RunOptions{CaptureStdout: true})
	if err != nil {
		return nil, err
	}
	var prs []ListOpenPRsData
	if err := json.Unmarshal([]byte(out), &prs); err != nil {
		return nil, errorx.Wrapf("+stacks", err, "parse gh pr list output: %s", out)
	}
	return prs, nil
}

func ensureLabelExists(
	ctx logx.LogCtx, repoRoot string, name string, color string, description string,
) error {
	assert.Precondition(repoRoot != "", "repoRoot must be non-empty")
	assert.Precondition(name != "", "name must be non-empty")
	assert.Precondition(color != "", "color must be non-empty")
	assert.Precondition(description != "", "description must be non-empty")

	listLabelsCmd := cmdx.New(
		"gh", "label", "list",
		"--search", name, "--json", "name", "--limit", "100",
	).In(repoRoot)
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
	createLabelCmd := cmdx.New(
		"gh", "label", "create", name,
		"--color", color, "--description", description,
	).In(repoRoot)
	_, err = createLabelCmd.Run(ctx, cmdx.RunOptionsDefault())
	return err
}

func formatPRBody(upstreamCommit string) string {
	return fmt.Sprintf("Pull in changes from upstream commit %s", upstreamCommit)
}

func formatMergeBody(metadata subtreeMetadata) string {
	lines := []string{
		fmt.Sprintf("Pull in changes from upstream commit %s", metadata.UpstreamCommit),
		"",
		mergebotSubtreeDirTrailer + ": " + metadata.Dir,
		mergebotLocalCommitTrailer + ": " + metadata.LocalCommit,
		mergebotUpstreamCommitTrailer + ": " + metadata.UpstreamCommit,
	}
	return strings.Join(lines, "\n")
}
