package meta_test

import (
	"os"
	"slices"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/typesanitizer/happygo/common/check"
	"github.com/typesanitizer/happygo/meta/internal/config"
)

func TestWorkspaceConfig(t *testing.T) {
	t.Parallel()

	h := check.New(t)

	f, err := os.Open("repo-configuration.json")
	h.NoErrorf(err, "opening repo-configuration.json")
	t.Cleanup(func() { _ = f.Close() })

	wsConfig, err := config.Load(f)
	h.NoErrorf(err, "loading repo configuration")

	var configFolders []string
	for folder := range wsConfig.ForkedFolders {
		configFolders = append(configFolders, folder)
	}
	sort.Strings(configFolders)

	t.Run("MainBranchCoverage", func(t *testing.T) {
		t.Parallel()
		h := check.New(t)

		for folder, repo := range map[string]config.GitHubRepo{
			"go":    "golang/go",
			"tools": "golang/tools",
			"delve": "go-delve/delve",
		} {
			forked, ok := wsConfig.ForkedFolders[folder]
			h.Assertf(ok, "missing forked folder %q", folder)
			h.Assertf(forked.GitHubRepo == repo, "forked folder %q has repo %q, want %q", folder, forked.GitHubRepo, repo)
		}

		mainMapping, ok := wsConfig.BranchMappings.ByLocalBranch["main"]
		h.Assertf(ok, "missing main branch mapping")

		for _, repo := range []config.GitHubRepo{"golang/go", "golang/tools", "go-delve/delve"} {
			_, ok := mainMapping.UpstreamMap.ByGitHubRepo[repo]
			h.Assertf(ok, "main mapping missing repo %q", repo)
		}
	})

	t.Run("WorkflowProjectChoices", func(t *testing.T) {
		t.Parallel()
		h := check.New(t)

		workflowBytes, err := os.ReadFile("../.github/workflows/upstream-sync.yml")
		h.NoErrorf(err, "reading upstream-sync.yml")

		var workflow struct {
			On struct {
				WorkflowDispatch struct {
					Inputs struct {
						Project struct {
							Options []string `yaml:"options"`
						} `yaml:"project"`
					} `yaml:"inputs"`
				} `yaml:"workflow_dispatch"`
			} `yaml:"on"`
		}
		h.NoErrorf(yaml.Unmarshal(workflowBytes, &workflow), "parsing upstream-sync.yml")

		yamlChoices := workflow.On.WorkflowDispatch.Inputs.Project.Options
		// Filter out "all" — it's a meta-choice, not a project folder.
		yamlFolders := slices.DeleteFunc(slices.Clone(yamlChoices), func(s string) bool { return s == "all" })
		sort.Strings(yamlFolders)

		h.Assertf(slices.Equal(configFolders, yamlFolders),
			"workflow project choices %v do not match repo-configuration.json folders %v", yamlFolders, configFolders)
	})

	// See SYNC(id: linter-exclusions).
	t.Run("LinterExclusions", func(t *testing.T) {
		t.Parallel()
		h := check.New(t)

		lintBytes, err := os.ReadFile("../.golangci.yml")
		h.NoErrorf(err, "reading .golangci.yml")

		var lintCfg struct {
			Linters struct {
				Exclusions struct {
					Paths []string `yaml:"paths"`
				} `yaml:"exclusions"`
			} `yaml:"linters"`
		}
		h.NoErrorf(yaml.Unmarshal(lintBytes, &lintCfg), "parsing .golangci.yml")

		var excludedFolders []string
		for _, p := range lintCfg.Linters.Exclusions.Paths {
			folder, ok := strings.CutPrefix(p, "^")
			if !ok {
				continue
			}
			folder, ok = strings.CutSuffix(folder, "/")
			if !ok {
				continue
			}
			excludedFolders = append(excludedFolders, folder)
		}
		sort.Strings(excludedFolders)

		h.Assertf(slices.Equal(excludedFolders, configFolders),
			"linter exclusions %v do not match repo-configuration.json folders %v", excludedFolders, configFolders)
	})
}
