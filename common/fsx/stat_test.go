package fsx_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"pgregory.net/rapid"

	"github.com/typesanitizer/happygo/common/check"
	. "github.com/typesanitizer/happygo/common/check/prelude"
	. "github.com/typesanitizer/happygo/common/core"
	"github.com/typesanitizer/happygo/common/core/pathx"
	"github.com/typesanitizer/happygo/common/core/pathx/pathx_testkit"
	"github.com/typesanitizer/happygo/common/errorx"
	"github.com/typesanitizer/happygo/common/fsx"
	"github.com/typesanitizer/happygo/common/syscaps"
)

func TestFSStat(t *testing.T) {
	h := check.New(t)

	h.Run("Unit", func(h check.Harness) {
		h.Parallel()

		parent := h.T().TempDir()
		target := filepath.Join(parent, "target")
		h.NoErrorf(os.Mkdir(target, 0o755), "Mkdir(%q)", target)

		link := filepath.Join(parent, "link")
		if err := os.Symlink(target, link); err != nil {
			h.T().Skipf("skipping: symlinks unavailable: %v", err)
		}

		repoFS := Do(syscaps.FS(NewAbsPath(link)))(h)

		followed := Do(repoFS.Stat(pathx.Dot(), fsx.StatOptions{
			FollowFinalSymlink:     true,
			OnErrorTraverseParents: false,
		}))(h)
		h.Assertf(followed.IsDir(), "Stat(., FollowFinalSymlink=true).IsDir() = false, want true")
		h.Assertf(followed.Mode()&os.ModeSymlink == 0,
			"Stat(., FollowFinalSymlink=true).Mode() = %v, want non-symlink", followed.Mode())

		notFollowed := Do(repoFS.Stat(pathx.Dot(), fsx.StatOptions{
			FollowFinalSymlink:     false,
			OnErrorTraverseParents: false,
		}))(h)
		h.Assertf(!notFollowed.IsDir(), "Stat(., FollowFinalSymlink=false).IsDir() = true, want false")
		h.Assertf(notFollowed.Mode()&os.ModeSymlink != 0,
			"Stat(., FollowFinalSymlink=false).Mode() = %v, want symlink", notFollowed.Mode())
	})

	h.Run("ShortestMissingProperties", func(h check.Harness) {
		h.Parallel()

		root := NewAbsPath(h.T().TempDir())
		componentsGen := rapid.SliceOfN(pathx_testkit.ComponentGen(), 1, 6)
		rapid.Check(h.T(), func(t *rapid.T) {
			h := check.NewBasic(t)
			components := componentsGen.Draw(t, "components")
			existingPrefixLen := rapid.IntRange(0, len(components)-1).Draw(t, "existingPrefixLen")
			followFinalSymlink := rapid.Bool().Draw(t, "followFinalSymlink")

			repoFS := Do(fsx.MemMap(root))(h)
			if existingPrefixLen > 0 {
				existingPrefix := joinedRelPath(components[:existingPrefixLen])
				h.NoErrorf(repoFS.MkdirAll(existingPrefix, 0o755), "MkdirAll(%q)", existingPrefix)
			}

			rel := joinedRelPath(components)
			opts := fsx.StatOptions{
				FollowFinalSymlink:     followFinalSymlink,
				OnErrorTraverseParents: false,
			}
			_, err := repoFS.Stat(rel, opts)
			statErr := requireStatError(h, err, rel, opts)
			check.AssertSame(h, rel, statErr.ShortestMissing(),
				"ShortestMissing() with OnErrorTraverseParents=false")

			opts.OnErrorTraverseParents = true
			_, err = repoFS.Stat(rel, opts)
			statErr = requireStatError(h, err, rel, opts)
			wantShortestMissing := joinedRelPath(components[:existingPrefixLen+1])
			check.AssertSame(h, wantShortestMissing, statErr.ShortestMissing(),
				"ShortestMissing() with OnErrorTraverseParents=true")
		})
	})
}

func joinedRelPath(components []string) RelPath {
	if len(components) == 0 {
		return pathx.Dot()
	}
	return NewRelPath(filepath.Join(components...))
}

func requireStatError(h check.BasicHarness, err error, rel RelPath, opts fsx.StatOptions) *fsx.StatError {
	call := fmt.Sprintf("Stat(%q, fsx.StatOptions{FollowFinalSymlink: %t, OnErrorTraverseParents: %t})",
		rel, opts.FollowFinalSymlink, opts.OnErrorTraverseParents)
	h.Assertf(err != nil, "%s unexpectedly succeeded", call)

	statErr, ok := errorx.FindInChainAs[*fsx.StatError](err).Get()
	h.Assertf(ok, "%s error type = %T, want chain containing *fsx.StatError", call, err)
	h.Assertf(errorx.GetRootCauseAsValue(err, fsx.ErrNotExist),
		"%s error %v does not satisfy root cause == fsx.ErrNotExist", call, err)
	return statErr
}
