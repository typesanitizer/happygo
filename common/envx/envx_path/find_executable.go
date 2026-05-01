// Package envx_path provides environment-sensitive PATH lookup helpers.
package envx_path

import (
	"fmt"
	"iter"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/typesanitizer/happygo/common/assert"
	"github.com/typesanitizer/happygo/common/core/pathx"
	"github.com/typesanitizer/happygo/common/envx"
	"github.com/typesanitizer/happygo/common/errorx"
	"github.com/typesanitizer/happygo/common/fsx"
)

// NotExecutableErr indicates a candidate file exists but is not executable.
var NotExecutableErr = errorx.New("nostack", "not an executable file")

// FindExecutable is analogous to exec.LookPath for bare executable names, but
// searches within fs instead of the host filesystem.
//
// Differences from exec.LookPath:
//   - PATH entries that resolve outside fs.Root() return an errorx.Problem
//     with code errorx.Code_AccessDenied instead of consulting the host
//     filesystem
//   - environment lookup comes from env instead of process-global state;
//
// Error cases:
//   - If PATH is missing or empty, the root cause is a [errorx.Problem] with
//     code [errorx.Code_InvalidArgument].
//   - If a relative PATH entry inside fs resolves to a matching executable,
//     the root cause is [ErrDot], like exec.LookPath.
//   - If a PATH entry resolves to a directory which does not fall under fs,
//     the root cause is a [errorx.Problem] with code [errorx.Code_AccessDenied].
//   - If no executable is found under a valid PATH, the root cause is
//     [os.ErrNotExist].
//
// When an error is returned, other metadata about the search/potential candidates
// is included via an [ExeSearchError] (discoverable via [errorx.FindInChainAs]),
// for additional context. It contains a list of IgnoredPaths.
//
//   - If some filesystem error was hit (e.g. permission denied), that error is
//     recorded as IgnoredPath.Err.
//   - This excludes "file not found" errors, because those are expected in routine
//     operation.
//   - If a file match was found, but it was not executable, IgnoredPath.Err is set
//     to [NotExecutableErr].
//
// Pre-condition: name is non-empty and does not contain path separators.
func FindExecutable(fs fsx.FS, env envx.Env, name string) (pathx.AbsPath, error) {
	assert.Preconditionf(name != "", "name is empty")
	assert.Preconditionf(!pathx.HasPathSeparators(name), "name contains path separators: %q", name)

	pathEnvVar, ok := env.Lookup("PATH").Get()
	if !ok || pathEnvVar == "" {
		return pathx.AbsPath{}, errorx.NewProblem(errorx.Code_InvalidArgument, "PATH must be set and non-empty")
	}
	var ignoredPaths []IgnoredPath
	for searchDir := range searchDirs(pathEnvVar) {
		var dir pathx.AbsPath
		if searchDir.isRelative {
			dir = fs.Root().Join(pathx.NewRelPath(searchDir.pathEntry))
		} else {
			dir = pathx.NewAbsPath(searchDir.pathEntry)
		}
		if !dir.MakeRelativeTo(fs.Root()).IsSome() {
			err := errorx.NewProblem(errorx.Code_AccessDenied,
				fmt.Sprintf("PATH entry %q resolves outside filesystem root %q", searchDir.pathEntry, fs.Root()))
			return pathx.AbsPath{}, wrapIgnored(err, ignoredPaths)
		}
		for candidatePath := range candidatePaths(env, dir.JoinComponents(name)) {
			rootRel, ok := candidatePath.MakeRelativeTo(fs.Root()).Get()
			// Proof of invariant:
			// 1. name doesn't have path separators (by pre-condition on FindExecutable).
			// 2. The check above returned early if dir escaped fs.Root().
			// 3. If dir is inside fs.Root() and name doesn't have path separators,
			//    then dir.JoinComponents(name) is inside fs.Root() as well.
			// 4. candidatePaths only potentially appends a file extension.
			assert.Invariantf(ok, "candidate path %q escaped filesystem root %q", candidatePath, fs.Root())
			info, statErr := fs.Stat(rootRel.Rel(), fsx.StatOptions{FollowFinalSymlink: true, OnErrorTraverseParents: false})
			if statErr != nil {
				if !errorx.GetRootCauseAsValue(statErr, fsx.ErrNotExist) {
					ignoredPaths = append(ignoredPaths, IgnoredPath{Path: candidatePath, Err: statErr})
				}
				continue
			}
			if info.IsDir() {
				continue
			}
			if !isExecutableCandidate(info.Mode()) {
				ignoredPaths = append(ignoredPaths, IgnoredPath{Path: candidatePath, Err: NotExecutableErr})
				continue
			}
			if searchDir.isRelative {
				err := errorx.Wrapf("nostack", ErrDot,
					"PATH entry %q resolved executable %s relative to filesystem root",
					searchDir.pathEntry, candidatePath)
				return candidatePath, wrapIgnored(err, ignoredPaths)
			}
			return candidatePath, nil
		}
	}
	err := errorx.Wrapf("nostack", os.ErrNotExist,
		"failed to find executable %q in PATH %q", name, pathEnvVar)
	return pathx.AbsPath{}, wrapIgnored(err, ignoredPaths)
}

// IgnoredPath records a candidate path that was found but skipped during
// executable search.
type IgnoredPath struct {
	Path pathx.AbsPath
	// Err is guaranteed to be non-nil.
	Err error
}

// ExeSearchError wraps an executable search error with information about
// candidates that were found but ignored (e.g. permission errors, non-executable files).
type ExeSearchError struct {
	inner        error         // guaranteed non-nil
	ignoredPaths []IgnoredPath // guaranteed non-empty
}

// Post-condition: The returned slice is guaranteed to be non-empty.
func (e *ExeSearchError) IgnoredPaths() []IgnoredPath {
	return e.ignoredPaths
}

func (e *ExeSearchError) Error() string {
	var b strings.Builder
	_, _ = fmt.Fprintf(&b, "%s (%d candidates ignored)", e.inner, len(e.ignoredPaths))
	for _, ignoredPath := range e.ignoredPaths {
		b.WriteString("\n")
		switch {
		case errorx.GetRootCauseAsValue(ignoredPath.Err, NotExecutableErr):
			_, _ = fmt.Fprintf(&b, "ignored %q as it was not executable", ignoredPath.Path)
		default:
			_, _ = fmt.Fprintf(&b, "ignored %q due to Stat error %v", ignoredPath.Path, ignoredPath.Err)
		}
	}
	return b.String()
}

func (e *ExeSearchError) Unwrap() error {
	return e.inner
}

type searchDir struct {
	pathEntry  string
	isRelative bool
}

func searchDirs(pathEnvVar string) iter.Seq[searchDir] {
	return func(yield func(searchDir) bool) {
		for pathEntry := range strings.SplitSeq(pathEnvVar, string(os.PathListSeparator)) {
			if pathEntry == "" {
				continue
			}
			if !yield(searchDir{pathEntry: pathEntry, isRelative: !filepath.IsAbs(pathEntry)}) {
				return
			}
		}
	}
}

// candidatePaths returns a sequence of all the paths to examine
// for executables, given a particular potential executable path.
//
// This largely matters for Windows because on Windows, the
// search needs to look at extensions inferred via the PATHEXT
// environment variable, in case base itself doesn't have an extension.
func candidatePaths(env envx.Env, base pathx.AbsPath) iter.Seq[pathx.AbsPath] {
	return func(yield func(pathx.AbsPath) bool) {
		if runtime.GOOS != "windows" || filepath.Ext(base.String()) != "" {
			yield(base)
			return
		}
		pathExt, ok := env.Lookup("PATHEXT").Get()
		if !ok || pathExt == "" {
			// Match exec.LookPath's Windows fallback when PATHEXT is unset or empty.
			pathExt = ".com;.exe;.bat;.cmd"
		}
		// Lowercase to match exec.LookPath behavior (see pathExt() in os/exec).
		for ext := range strings.SplitSeq(strings.ToLower(pathExt), ";") {
			if ext == "" {
				continue
			}
			if !yield(base.AppendExtension(ext)) {
				return
			}
		}
	}
}

func wrapIgnored(err error, ignoredPaths []IgnoredPath) error {
	if len(ignoredPaths) == 0 {
		return err
	}
	return &ExeSearchError{inner: err, ignoredPaths: ignoredPaths}
}

// On Unix, executable bits gate path lookup results. On Windows, this matches
// exec.LookPath: after PATHEXT filtering, any non-directory file is accepted
// here because Windows path lookup does not use Unix-style execute mode bits.
func isExecutableCandidate(mode os.FileMode) bool {
	if runtime.GOOS == "windows" {
		return true
	}
	return mode&0o111 != 0
}
