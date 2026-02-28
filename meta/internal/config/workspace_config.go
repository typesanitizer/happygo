package config

import "github.com/typesanitizer/happygo/common/errorx"

// UpstreamForProject resolves the upstream repo and branch config for one project on a local branch.
func (wc WorkspaceConfig) UpstreamForProject(localBranch string, project string) (UpstreamRepo, error) {
	mapping, ok := wc.BranchMappings.ByLocalBranch[localBranch]
	if !ok {
		return UpstreamRepo{}, errorx.Newf("nostack", "no branch mapping configured for branch %q", localBranch)
	}

	forkedFolder, ok := wc.ForkedFolders[project]
	if !ok {
		return UpstreamRepo{}, errorx.Newf("nostack", "project %q is not a forked folder", project)
	}

	upstream, ok := mapping.UpstreamMap.ByGitHubRepo[forkedFolder.GitHubRepo]
	if !ok {
		return UpstreamRepo{}, errorx.Newf("nostack", "project %q is not configured for branch %q", project, localBranch)
	}
	return upstream, nil
}
