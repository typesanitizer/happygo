package fsx

import (
	"os"

	. "github.com/typesanitizer/happygo/common/core"
	"github.com/typesanitizer/happygo/common/core/pathx"
)

// Stat returns file info for the given root-relative path.
//
// If rel corresponds to a symlink:
//   - If opts.FollowFinalSymlink is true, the symlink is dereferenced,
//     and the os.FileInfo for the path it points to is returned.
//   - If opts.FollowFinalSymlink is false, the os.FileInfo for the
//     symlink itself is returned.
//
// On error, the returned error is always a *StatError. If
// opts.OnErrorTraverseParents is true, StatError.ShortestMissing() returns the
// shallowest ancestor that does not exist. Otherwise, ShortestMissing()
// returns rel.
func (fs FS) Stat(rel RelPath, opts StatOptions) (os.FileInfo, error) {
	var info os.FileInfo
	var err error
	if opts.FollowFinalSymlink {
		info, err = fs.base.Stat(rel.String())
	} else {
		info, _, err = fs.base.LstatIfPossible(rel.String())
	}
	if err == nil {
		return info, nil
	}
	if !opts.OnErrorTraverseParents {
		return nil, &StatError{fsError: err, shortestMissing: rel}
	}
	// Binary search over path components to find the shallowest missing
	// ancestor with O(log n) stat calls. Since RelPath is normalized, we can
	// slice the raw string at separator boundaries without re-joining.
	raw := rel.String()
	// Collect separator indices; each marks the end of a prefix that is a
	// valid ancestor path.
	var seps []int
	for i := 0; i < len(raw); i++ {
		if pathx.IsPathSeparator(raw[i]) {
			seps = append(seps, i)
		}
	}
	// seps[i] gives prefix raw[:seps[i]] which is an ancestor.
	// We want the smallest index where stat fails.
	// Invariant: raw[:lo] exists (lo=0 means root itself), raw[:hi] does not.
	lo, hi := 0, len(seps)
	for lo < hi {
		mid := (lo + hi) / 2
		if _, statErr := fs.base.Stat(raw[:seps[mid]]); statErr != nil {
			hi = mid
		} else {
			lo = mid + 1
		}
	}
	if lo < len(seps) {
		return nil, &StatError{fsError: err, shortestMissing: NewRelPath(raw[:seps[lo]])}
	}
	// All ancestors exist; the leaf itself is the shallowest missing.
	return nil, &StatError{fsError: err, shortestMissing: rel}
}

type StatOptions struct {
	// FollowFinalSymlink controls behavior when the final path component is a
	// symlink. If true, the symlink is dereferenced (stat). If false, info about
	// the symlink itself is returned (lstat). Intervening symlinks in parent
	// directories are always resolved by the OS regardless of this setting.
	FollowFinalSymlink bool
	// OnErrorTraverseParents walks up the path to find the shallowest missing
	// ancestor when the initial stat fails. StatError.ShortestMissing() will
	// return the shallowest component that does not exist.
	OnErrorTraverseParents bool
}

// StatError holds information about a failed stat, including the shallowest
// missing ancestor when OnErrorTraverseParents was set.
type StatError struct {
	fsError         error
	shortestMissing RelPath
}

func (e *StatError) Error() string {
	return e.fsError.Error()
}

func (e *StatError) Unwrap() error {
	return e.fsError
}

// ShortestMissing returns the shallowest missing path component. If
// OnErrorTraverseParents was not set, this equals the original path.
func (e *StatError) ShortestMissing() RelPath {
	return e.shortestMissing
}
