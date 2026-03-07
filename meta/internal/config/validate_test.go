package config

import (
	"strings"
	"testing"

	"github.com/typesanitizer/happygo/common/check"
)

func validConfig() WorkspaceConfigJSON {
	return WorkspaceConfigJSON{
		ForkedFolders: []ForkedFolderJSON{
			{Folder: "go", GitHubProject: "golang/go"},
			{Folder: "tools", GitHubProject: "golang/tools"},
		},
		BranchMappings: []BranchMappingJSON{
			{
				LocalBranch: "main",
				Upstream: []UpstreamRepoJSON{
					{GitHubProject: "golang/go", Branch: "master"},
					{GitHubProject: "golang/tools", Branch: "master"},
				},
			},
		},
	}
}

func TestValidateErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		modify  func(*WorkspaceConfigJSON)
		wantErr string
	}{
		{
			name:    "empty forked_folders",
			modify:  func(c *WorkspaceConfigJSON) { c.ForkedFolders = nil },
			wantErr: "forked_folders list is empty",
		},
		{
			name: "empty folder name",
			modify: func(c *WorkspaceConfigJSON) {
				c.ForkedFolders[0].Folder = ""
			},
			wantErr: "empty folder",
		},
		{
			name: "duplicate folder",
			modify: func(c *WorkspaceConfigJSON) {
				c.ForkedFolders = append(c.ForkedFolders, ForkedFolderJSON{Folder: "go", GitHubProject: "other/repo"})
			},
			wantErr: "duplicate folder",
		},
		{
			name: "invalid gh_project in forked_folders",
			modify: func(c *WorkspaceConfigJSON) {
				c.ForkedFolders[0].GitHubProject = "noslash"
			},
			wantErr: "invalid gh_project",
		},
		{
			name: "duplicate gh_project",
			modify: func(c *WorkspaceConfigJSON) {
				c.ForkedFolders[1].GitHubProject = "golang/go"
			},
			wantErr: "duplicate gh_project",
		},
		{
			name:    "empty branch_mappings",
			modify:  func(c *WorkspaceConfigJSON) { c.BranchMappings = nil },
			wantErr: "branch_mappings list is empty",
		},
		{
			name: "empty local branch",
			modify: func(c *WorkspaceConfigJSON) {
				c.BranchMappings[0].LocalBranch = ""
			},
			wantErr: "empty local branch",
		},
		{
			name: "empty upstream list",
			modify: func(c *WorkspaceConfigJSON) {
				c.BranchMappings[0].Upstream = nil
			},
			wantErr: "empty upstream list",
		},
		{
			name: "empty upstream list does not cause missing upstream",
			modify: func(c *WorkspaceConfigJSON) {
				c.BranchMappings[0].Upstream = nil
			},
			wantErr: "!missing upstream project",
		},
		{
			name: "duplicate local branch",
			modify: func(c *WorkspaceConfigJSON) {
				c.BranchMappings = append(c.BranchMappings, c.BranchMappings[0])
			},
			wantErr: "duplicate local branch",
		},
		{
			name: "invalid upstream gh_project",
			modify: func(c *WorkspaceConfigJSON) {
				c.BranchMappings[0].Upstream[0].GitHubProject = "bad"
			},
			wantErr: "invalid gh_project",
		},
		{
			name: "empty upstream branch",
			modify: func(c *WorkspaceConfigJSON) {
				c.BranchMappings[0].Upstream[0].Branch = ""
			},
			wantErr: "empty branch",
		},
		{
			name: "duplicate upstream project",
			modify: func(c *WorkspaceConfigJSON) {
				c.BranchMappings[0].Upstream = append(c.BranchMappings[0].Upstream,
					UpstreamRepoJSON{GitHubProject: "golang/go", Branch: "release"})
			},
			wantErr: "duplicate upstream project",
		},
		{
			name: "reference to non-forked project",
			modify: func(c *WorkspaceConfigJSON) {
				c.BranchMappings[0].Upstream[0].GitHubProject = "unknown/repo"
			},
			wantErr: "non-forked project",
		},
		{
			name: "missing upstream project",
			modify: func(c *WorkspaceConfigJSON) {
				c.BranchMappings[0].Upstream = c.BranchMappings[0].Upstream[:1]
			},
			wantErr: "missing upstream project",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := check.New(t)

			cfg := validConfig()
			tt.modify(&cfg)
			_, err := cfg.Validate()

			// A "!" prefix means the substring must be absent from the error.
			needle, wantAbsent := strings.CutPrefix(tt.wantErr, "!")

			if wantAbsent {
				h.Assertf(err == nil || !strings.Contains(err.Error(), needle), "got unwanted error substring %q in: %v", needle, err)
				return
			}

			h.Assertf(err != nil, "expected error containing %q, got nil", needle)
			h.Assertf(strings.Contains(err.Error(), needle), "error %q does not contain %q", err.Error(), needle)
		})
	}
}

func TestValidateSuccess(t *testing.T) {
	t.Parallel()
	h := check.New(t)

	cfg := validConfig()
	rc, err := cfg.Validate()
	h.NoErrorf(err, "validate config")
	h.Assertf(len(rc.ForkedFolders) == 2, "expected 2 forked folders, got %d", len(rc.ForkedFolders))
	h.Assertf(len(rc.BranchMappings.ByLocalBranch) == 1, "expected 1 branch mapping, got %d", len(rc.BranchMappings.ByLocalBranch))
}
