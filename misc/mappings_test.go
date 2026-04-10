package misc_test

import (
	"maps"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/typesanitizer/happygo/common/check"
	. "github.com/typesanitizer/happygo/common/check/prelude"
	"github.com/typesanitizer/happygo/common/collections"
	"github.com/typesanitizer/happygo/common/core/pathx"
	"github.com/typesanitizer/happygo/common/iterx"
	"github.com/typesanitizer/happygo/misc/internal/config"
)

func TestWorkspaceConfig(t *testing.T) {
	h := check.New(t)
	h.Parallel()

	f := DoMsg(os.Open("repo-configuration.json"))(h, "opening repo-configuration.json")
	t.Cleanup(func() { _ = f.Close() })

	wsConfig := Do(config.Load(f))(h)

	configFolders := collections.SortedMapKeys(wsConfig.ForkedFolders)

	repoRoot := DoMsg(pathx.ResolveAbsPath(".."))(h, "resolving repo root").String()

	forkedProjects := map[string]config.GitHubRepo{
		"go":    "golang/go",
		"tools": "golang/tools",
		"delve": "go-delve/delve",
	}

	h.Run("MainBranchCoverage", func(h check.Harness) {
		h.Parallel()

		for folder, repo := range forkedProjects {
			forked, ok := wsConfig.ForkedFolders[folder]
			h.Assertf(ok, "forked folder %q must be present in repo-configuration.json", folder)
			h.Assertf(forked.GitHubRepo == repo,
				"forked folder %q has repo %q, want %q", folder, forked.GitHubRepo, repo)
		}

		mainMapping, ok := wsConfig.BranchMappings.ByLocalBranch["main"]
		h.Assertf(ok, "main branch mapping must be present in repo-configuration.json")

		for _, repo := range iterx.Collect(maps.Values(forkedProjects)) {
			_, ok := mainMapping.UpstreamMap.ByGitHubRepo[repo]
			h.Assertf(ok, "main branch mapping must include upstream repo %q", repo)
		}
	})

	h.Run("WorkflowProjectChoices", func(h check.Harness) {
		h.Parallel()

		workflowBytes := DoMsg(os.ReadFile(filepath.Join(repoRoot, ".github/workflows/upstream-sync.yml")))(h,
			"reading upstream-sync.yml")

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
		yamlFolders := collections.FilterSlice(yamlChoices, func(s string) bool { return s != "all" })
		sort.Strings(yamlFolders)

		check.AssertSame(h, configFolders, yamlFolders,
			"each forked project must be listed as a choice in GHA workflow dispatch")
	})

	// See SYNC(id: linter-exclusions).
	h.Run("LinterExclusions", func(h check.Harness) {
		h.Parallel()

		lintBytes := DoMsg(os.ReadFile(filepath.Join(repoRoot, ".golangci.yml")))(h,
			"reading .golangci.yml")

		var lintCfg struct {
			Linters struct {
				Exclusions struct {
					Paths []string `yaml:"paths"`
				} `yaml:"exclusions"`
			} `yaml:"linters"`
		}
		h.NoErrorf(yaml.Unmarshal(lintBytes, &lintCfg), "parsing .golangci.yml")

		configSet := collections.NewSet[string]()
		for _, f := range configFolders {
			configSet.InsertNew(f)
		}

		excludedSet := collections.NewSet[string]()
		for _, p := range lintCfg.Linters.Exclusions.Paths {
			folder, ok := strings.CutPrefix(p, "^")
			h.Assertf(ok, "linter exclusion path %q must start with ^", p)
			folder, ok = strings.CutSuffix(folder, "/")
			h.Assertf(ok, "linter exclusion path %q must end with /", p)
			excludedSet.InsertNew(folder)
		}

		h.Assertf(excludedSet.IsSubsetOf(&configSet),
			"linter exclusions should only exclude forked projects")
	})
}
