package config

import (
	"github.com/typesanitizer/happygo/common/collections"
	"github.com/typesanitizer/happygo/common/errorx"
)

// Validate checks structural invariants for JSON workspace configuration and builds validated config.
func (wcJSON WorkspaceConfigJSON) Validate() (WorkspaceConfig, error) {
	forkedFolders, forkedRepos, err := validateForkedFolders(wcJSON.ForkedFolders)
	if err != nil {
		return WorkspaceConfig{}, err
	}
	branchMappings, err := validateBranchMappings(wcJSON.BranchMappings, forkedRepos)
	if err != nil {
		return WorkspaceConfig{}, err
	}
	return WorkspaceConfig{ForkedFolders: forkedFolders, BranchMappings: branchMappings}, nil
}

func validateForkedFolders(forkedFoldersJSON []ForkedFolderJSON) (map[string]ForkedFolder, collections.Set[GitHubRepo], error) {
	forkedFolders := map[string]ForkedFolder{}
	forkedRepos := collections.NewSet[GitHubRepo]()
	var err error

	if len(forkedFoldersJSON) == 0 {
		err = errorx.Join(err, errorx.New("nostack", "forked_folders list is empty"))
	}

	for _, forkedFolderJSON := range forkedFoldersJSON {
		if forkedFolderJSON.Folder == "" {
			err = errorx.Join(err, errorx.New("nostack", "forked_folders has empty folder"))
			continue
		}
		if _, ok := forkedFolders[forkedFolderJSON.Folder]; ok {
			err = errorx.Join(err, errorx.Newf("nostack", "forked_folders has duplicate folder %q", forkedFolderJSON.Folder))
			continue
		}

		githubRepo, parseErr := parseGitHubRepo(forkedFolderJSON.GitHubProject)
		if parseErr != nil {
			err = errorx.Join(err, errorx.Wrapf("nostack", parseErr, "forked_folders[%q]", forkedFolderJSON.Folder))
			continue
		}
		if !forkedRepos.Insert(githubRepo) {
			err = errorx.Join(err, errorx.Newf("nostack", "forked_folders has duplicate gh_project %q", githubRepo))
			continue
		}

		forkedFolders[forkedFolderJSON.Folder] = ForkedFolder{
			Folder:     forkedFolderJSON.Folder,
			GitHubRepo: githubRepo,
		}
	}

	if err != nil {
		return nil, forkedRepos, err
	}
	return forkedFolders, forkedRepos, nil
}

func validateBranchMappings(mappingsJSON []BranchMappingJSON, forkedRepos collections.Set[GitHubRepo]) (BranchMappings, error) {
	mappings := BranchMappings{ByLocalBranch: map[string]BranchMapping{}}
	var err error

	if len(mappingsJSON) == 0 {
		err = errorx.Join(err, errorx.New("nostack", "branch_mappings list is empty"))
	}

	for _, mappingJSON := range mappingsJSON {
		if mappingJSON.LocalBranch == "" {
			err = errorx.Join(err, errorx.New("nostack", "branch mapping has empty local branch"))
			continue
		}
		if len(mappingJSON.Upstream) == 0 {
			err = errorx.Join(err, errorx.Newf("nostack", "branch mapping %q has empty upstream list", mappingJSON.LocalBranch))
			continue
		}
		if _, ok := mappings.ByLocalBranch[mappingJSON.LocalBranch]; ok {
			err = errorx.Join(err, errorx.Newf("nostack", "duplicate local branch mapping: %q", mappingJSON.LocalBranch))
			continue
		}

		upstreamMap := UpstreamMap{ByGitHubRepo: map[GitHubRepo]UpstreamRepo{}}
		for _, upstreamJSON := range mappingJSON.Upstream {
			githubRepo, parseErr := parseGitHubRepo(upstreamJSON.GitHubProject)
			if parseErr != nil {
				err = errorx.Join(err, errorx.Wrapf("nostack", parseErr, "branch mapping %q", mappingJSON.LocalBranch))
				continue
			}
			if upstreamJSON.Branch == "" {
				err = errorx.Join(err, errorx.Newf("nostack", "branch mapping %q has upstream %q with empty branch", mappingJSON.LocalBranch, upstreamJSON.GitHubProject))
				continue
			}
			if _, ok := upstreamMap.ByGitHubRepo[githubRepo]; ok {
				err = errorx.Join(err, errorx.Newf("nostack", "branch mapping %q has duplicate upstream project %q", mappingJSON.LocalBranch, githubRepo))
				continue
			}
			if !forkedRepos.Contains(githubRepo) {
				err = errorx.Join(err, errorx.Newf("nostack", "branch mapping %q references non-forked project %q", mappingJSON.LocalBranch, githubRepo))
				continue
			}
			upstreamMap.ByGitHubRepo[githubRepo] = UpstreamRepo{
				GitHubRepo: githubRepo,
				Branch:     upstreamJSON.Branch,
			}
		}

		for forkedRepo := range forkedRepos.ValuesNonDet() {
			if _, ok := upstreamMap.ByGitHubRepo[forkedRepo]; !ok {
				err = errorx.Join(err, errorx.Newf("nostack", "branch mapping %q missing upstream project %q", mappingJSON.LocalBranch, forkedRepo))
			}
		}

		mappings.ByLocalBranch[mappingJSON.LocalBranch] = BranchMapping{
			LocalBranch: mappingJSON.LocalBranch,
			UpstreamMap: upstreamMap,
		}
	}

	if err != nil {
		return BranchMappings{}, err
	}
	return mappings, nil
}
