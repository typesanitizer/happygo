package config

import (
	"strings"

	"github.com/typesanitizer/happygo/common/errorx"
)

func parseGitHubRepo(s string) (GitHubRepo, error) {
	owner, repo, ok := strings.Cut(s, "/")
	if !ok || owner == "" || repo == "" || strings.Contains(repo, "/") {
		return "", errorx.Newf("nostack", "invalid gh_project %q: want <owner>/<repo>", s)
	}
	return GitHubRepo(s), nil
}
