package config

import (
	"encoding/json"
	"io"

	"github.com/typesanitizer/happygo/common/errorx"
	"github.com/typesanitizer/happygo/common/fsx"
)

// WorkspaceConfig is the validated in-memory repository configuration.
type WorkspaceConfig struct {
	// ForkedFolders maps folder name to forked folder metadata. Always non-empty.
	ForkedFolders map[fsx.Name]ForkedFolder
	// BranchMappings is the validated in-memory branch mapping representation.
	BranchMappings BranchMappings
}

// Load reads workspace configuration from JSON and validates it.
func Load(r io.Reader) (WorkspaceConfig, error) {
	var wcJSON WorkspaceConfigJSON
	if err := json.NewDecoder(r).Decode(&wcJSON); err != nil {
		return WorkspaceConfig{}, errorx.Wrapf("+stacks", err, "parse workspace configuration")
	}
	wc, err := wcJSON.Validate()
	if err != nil {
		return WorkspaceConfig{}, err
	}
	return wc, nil
}

// UpstreamForProject resolves the upstream repo and branch config for one project on a local branch.
func (wc WorkspaceConfig) UpstreamForProject(localBranch string, project fsx.Name) (UpstreamRepo, error) {
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
