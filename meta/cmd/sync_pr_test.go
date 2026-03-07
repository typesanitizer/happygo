package main

import (
	"strings"
	"testing"

	"github.com/typesanitizer/happygo/common/check"
)

func TestSyncMergeBodyAllTrailers(t *testing.T) {
	t.Parallel()
	h := check.New(t)

	metadata := subtreeMetadata{
		Dir:            "go",
		LocalCommit:    "abc123",
		UpstreamCommit: "def456",
	}
	body := syncMergeBody(metadata)

	h.Assertf(strings.Contains(body, mergebotSubtreeDirTrailer+": go"), "missing %s trailer:\n%s", mergebotSubtreeDirTrailer, body)
	h.Assertf(strings.Contains(body, mergebotLocalCommitTrailer+": abc123"), "missing %s trailer:\n%s", mergebotLocalCommitTrailer, body)
	h.Assertf(strings.Contains(body, mergebotUpstreamCommitTrailer+": def456"), "missing %s trailer:\n%s", mergebotUpstreamCommitTrailer, body)
	h.Assertf(!strings.Contains(body, "git-subtree-"), "body should not contain legacy git-subtree-* trailers:\n%s", body)
}
