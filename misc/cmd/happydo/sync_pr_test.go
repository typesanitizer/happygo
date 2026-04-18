package main

import (
	"strings"
	"testing"

	"github.com/typesanitizer/happygo/common/check"
	"github.com/typesanitizer/happygo/common/fsx"
)

func TestSyncMergeBodyAllTrailers(t *testing.T) {
	h := check.New(t)
	h.Parallel()

	metadata := subtreeMetadata{
		Dir:            fsx.NewName("go"),
		LocalCommit:    "abc123",
		UpstreamCommit: "def456",
	}
	body := formatMergeBody(metadata)

	h.Assertf(strings.Contains(body, mergebotSubtreeDirTrailer+": go"), "missing %s trailer:\n%s", mergebotSubtreeDirTrailer, body)
	h.Assertf(strings.Contains(body, mergebotLocalCommitTrailer+": abc123"), "missing %s trailer:\n%s", mergebotLocalCommitTrailer, body)
	h.Assertf(strings.Contains(body, mergebotUpstreamCommitTrailer+": def456"), "missing %s trailer:\n%s", mergebotUpstreamCommitTrailer, body)
	h.Assertf(!strings.Contains(body, "git-subtree-"), "body should not contain legacy git-subtree-* trailers:\n%s", body)
}
