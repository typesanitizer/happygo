package envx_path_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/typesanitizer/happygo/common/assert"
	"github.com/typesanitizer/happygo/common/check"
	. "github.com/typesanitizer/happygo/common/check/prelude"
	. "github.com/typesanitizer/happygo/common/core"
	"github.com/typesanitizer/happygo/common/envx"
	"github.com/typesanitizer/happygo/common/envx/envx_path"
	"github.com/typesanitizer/happygo/common/errorx"
	"github.com/typesanitizer/happygo/common/fsx"
	"github.com/typesanitizer/happygo/common/fsx/fsx_testkit"
	"github.com/typesanitizer/happygo/common/iterx"
	"github.com/typesanitizer/happygo/common/syscaps"
)

func TestFindExecutable(t *testing.T) {
	h := check.New(t)
	// No h.Parallel() here because some nested tests use Setenv and Chdir.

	h.Run("Happy path works", testFindExecutableHappyPath)
	h.Run("Error cases are reported", testFindExecutableErrorCases)
	h.Run("Preconditions are enforced", testFindExecutablePreconditions)
}

func testFindExecutableHappyPath(h check.Harness) {
	exe := hostOSExecutable()
	fakeRoot := fsx_testkit.FakeRoot().JoinComponents("find-executable", "happy-path")

	h.Run("PATH behavior", func(h check.Harness) {
		fs := newMemFS(h, fakeRoot.JoinComponents("path-behavior"))
		binRel := NewRelPath("bin")
		h.NoErrorf(fs.MkdirAll(binRel, 0o755), "MkdirAll(%q)", binRel)
		exeRel := binRel.JoinComponents(exe.Name)
		writeExecutable(h, fs, exeRel)

		h.Run("Absolute PATH finds executable", func(h check.Harness) {
			h.Parallel()

			envVars := map[string]string{"PATH": fs.Root().Join(binRel).String()}
			if runtime.GOOS == "windows" {
				envVars["PATHEXT"] = ".com;.exe;.bat;.cmd"
			}
			env := testEnv(envVars)

			got := Do(envx_path.FindExecutable(fs, env, exe.LookupName))(h)
			check.AssertSame(h, fs.Root().Join(exeRel), got, "FindExecutable")
		})

		h.Run("Missing candidates are skipped", func(h check.Harness) {
			h.Parallel()

			envVars := map[string]string{
				"PATH": strings.Join([]string{
					fs.Root().JoinComponents("missing").String(),
					fs.Root().Join(binRel).String(),
				}, string(os.PathListSeparator)),
			}
			if runtime.GOOS == "windows" {
				envVars["PATHEXT"] = ".com;.exe;.bat;.cmd"
			}
			env := testEnv(envVars)

			got := Do(envx_path.FindExecutable(fs, env, exe.LookupName))(h)
			check.AssertSame(h, fs.Root().Join(exeRel), got, "FindExecutable after missing candidate")
		})
	})

	h.Run("Windows compatibility matches exec.LookPath", func(h check.Harness) {
		if runtime.GOOS != "windows" {
			h.T().Skip("Windows-specific exec.LookPath compatibility tests")
		}

		root := NewAbsPath(h.T().TempDir())
		fs := Do(syscaps.FS(root))(h)
		binRel := NewRelPath("bin")
		binDir := root.Join(binRel)
		h.NoErrorf(fs.MkdirAll(binRel, 0o755), "MkdirAll(%q)", binRel)

		h.Run("Empty PATHEXT falls back like exec.LookPath", func(h check.Harness) {
			exeRel := binRel.JoinComponents("tool.exe")
			writeExecutable(h, fs, exeRel)
			exePath := root.Join(exeRel)

			h.T().Setenv("PATH", binDir.String())
			h.T().Setenv("PATHEXT", "")

			wantPath := Do(exec.LookPath("tool"))(h)
			check.AssertSame(h, exePath.String(), wantPath, "exec.LookPath path")

			env := testEnv(map[string]string{"PATH": binDir.String()})
			gotPath := Do(envx_path.FindExecutable(fs, env, "tool"))(h)
			check.AssertSame(h, exePath, gotPath, "FindExecutable path")
		})

		h.Run("Explicit extension matches exec.LookPath", func(h check.Harness) {
			txtRel := binRel.JoinComponents("tool.txt")
			writeExecutable(h, fs, txtRel)
			txtPath := root.Join(txtRel)

			h.T().Setenv("PATH", binDir.String())

			wantPath := Do(exec.LookPath("tool.txt"))(h)
			check.AssertSame(h, txtPath.String(), wantPath, "exec.LookPath path")

			env := testEnv(map[string]string{"PATH": binDir.String()})
			gotPath := Do(envx_path.FindExecutable(fs, env, "tool.txt"))(h)
			check.AssertSame(h, txtPath, gotPath, "FindExecutable path")
		})
	})
}

func testFindExecutableErrorCases(h check.Harness) {
	exe := hostOSExecutable()
	fakeRoot := fsx_testkit.FakeRoot().JoinComponents("find-executable", "error-cases")

	h.Run("Missing PATH is invalid", func(h check.Harness) {
		h.Parallel()

		fs := newMemFS(h, fakeRoot.JoinComponents("missing-path"))
		_, err := envx_path.FindExecutable(fs, envx.Empty(), "tool")
		assertProblemCode(h, err, errorx.Code_InvalidArgument)
	})

	h.Run("Absolute PATH outside root is denied", func(h check.Harness) {
		h.Parallel()

		fs := newMemFS(h, fakeRoot.JoinComponents("absolute-outside-root"))
		outsideDir := fsx_testkit.FakeRoot().JoinComponents("outside", "absolute")
		env := testEnv(map[string]string{"PATH": outsideDir.String()})

		_, err := envx_path.FindExecutable(fs, env, "tool")
		assertProblemCode(h, err, errorx.Code_AccessDenied)
	})

	h.Run("Non-executable candidates are reported", func(h check.Harness) {
		h.Parallel()

		if runtime.GOOS == "windows" {
			h.T().Skip("Windows treats any non-directory file as executable after PATHEXT filtering")
		}

		fs := newMemFS(h, fakeRoot.JoinComponents("non-executable-candidates"))
		binRel := NewRelPath("bin")
		h.NoErrorf(fs.MkdirAll(binRel, 0o755), "MkdirAll(%q)", binRel)
		exeRel := binRel.JoinComponents(exe.Name)
		h.NoErrorf(fs.WriteFile(exeRel, []byte("#!/bin/sh\n"), 0o644), "WriteFile(%q)", exeRel)

		env := testEnv(map[string]string{"PATH": fs.Root().Join(binRel).String()})
		_, err := envx_path.FindExecutable(fs, env, exe.LookupName)
		h.Assertf(errorx.GetRootCauseAsValue(err, os.ErrNotExist),
			"expected not-exist error, got %v", err)

		searchErr, ok := errorx.FindInChainAs[*envx_path.ExeSearchError](err).Get()
		h.Assertf(ok, "expected *ExeSearchError, got %T", err)
		check.AssertSame(h, 1, len(searchErr.IgnoredPaths()), "IgnoredPaths count")
		ignored := searchErr.IgnoredPaths()[0]
		check.AssertSame(h, fs.Root().Join(exeRel), ignored.Path, "IgnoredPath.Path")
		h.Assertf(errorx.GetRootCauseAsValue(ignored.Err, envx_path.NotExecutableErr),
			"expected not-executable error, got %v", ignored.Err)

		wantErr := fmt.Sprintf("%s (%d candidates ignored)\nignored %q as it was not executable",
			searchErr.Unwrap().Error(), len(searchErr.IgnoredPaths()), ignored.Path)
		check.AssertSame(h, wantErr, searchErr.Error(), "ExeSearchError.Error()")
	})

	h.Run("Relative PATH outside root is denied", func(h check.Harness) {
		h.Parallel()

		fs := newMemFS(h, fakeRoot.JoinComponents("relative-outside-root"))
		outsideRel := filepath.Join("..", "outside")
		env := testEnv(map[string]string{"PATH": outsideRel})

		_, err := envx_path.FindExecutable(fs, env, "tool")
		assertProblemCode(h, err, errorx.Code_AccessDenied)
	})

	h.Run("exec.LookPath compatibility", func(h check.Harness) {
		root := NewAbsPath(h.T().TempDir())
		fs := Do(syscaps.FS(root))(h)
		binRel := NewRelPath("bin")
		binDir := root.Join(binRel)
		h.NoErrorf(fs.MkdirAll(binRel, 0o755), "MkdirAll(%q)", binRel)
		exeRel := binRel.JoinComponents(exe.Name)
		writeExecutable(h, fs, exeRel)

		h.Run("Relative PATH returns ErrDot", func(h check.Harness) {
			h.T().Chdir(root.String())
			h.T().Setenv("PATH", "bin")
			if runtime.GOOS == "windows" {
				h.T().Setenv("PATHEXT", ".com;.exe;.bat;.cmd")
			}

			wantPath, wantErr := exec.LookPath(exe.LookupName)
			h.Assertf(errorx.GetRootCauseAsValue(wantErr, exec.ErrDot),
				"expected exec.LookPath error %v to wrap exec.ErrDot", wantErr)
			check.AssertSame(h, filepath.Join("bin", exe.Name), wantPath, "exec.LookPath path")

			envVars := map[string]string{"PATH": "bin"}
			if runtime.GOOS == "windows" {
				envVars["PATHEXT"] = ".com;.exe;.bat;.cmd"
			}
			env := testEnv(envVars)

			gotPath, gotErr := envx_path.FindExecutable(fs, env, exe.LookupName)
			h.Assertf(errorx.GetRootCauseAsValue(gotErr, envx_path.ErrDot),
				"expected FindExecutable error %v to wrap ErrDot", gotErr)
			check.AssertSame(h, root.Join(exeRel), gotPath, "FindExecutable path")
		})

		h.Run("Stat errors are skipped", func(h check.Harness) {
			if runtime.GOOS == "windows" {
				h.T().Skip("permission-denied stat behavior differs on Windows")
			}

			h.NoErrorf(os.Chmod(binDir.String(), 0o000), "Chmod(%q)", binDir)
			h.T().Cleanup(func() {
				_ = os.Chmod(binDir.String(), 0o755)
			})

			h.T().Setenv("PATH", binDir.String())

			_, stdlibErr := exec.LookPath(exe.LookupName)
			h.Assertf(errorx.GetRootCauseAsValue(stdlibErr, exec.ErrNotFound),
				"expected exec.LookPath ErrNotFound, got %v", stdlibErr)

			env := testEnv(map[string]string{"PATH": binDir.String()})
			_, err := envx_path.FindExecutable(fs, env, exe.LookupName)
			h.Assertf(errorx.GetRootCauseAsValue(err, os.ErrNotExist),
				"expected not-exist error, got %v", err)

			searchErr, ok := errorx.FindInChainAs[*envx_path.ExeSearchError](err).Get()
			h.Assertf(ok, "expected *ExeSearchError, got %T", err)
			check.AssertSame(h, 1, len(searchErr.IgnoredPaths()), "IgnoredPaths count")
			ignored := searchErr.IgnoredPaths()[0]
			check.AssertSame(h, binDir.JoinComponents(exe.Name), ignored.Path, "IgnoredPath.Path")
			h.Assertf(errorx.GetRootCauseAsValue(ignored.Err, os.ErrPermission),
				"expected permission error, got %v", ignored.Err)

			wantErr := fmt.Sprintf("%s (%d candidates ignored)\nignored %q due to Stat error %v",
				searchErr.Unwrap().Error(), 1, ignored.Path, ignored.Err)
			check.AssertSame(h, wantErr, searchErr.Error(), "ExeSearchError.Error()")
		})
	})
}

func testFindExecutablePreconditions(h check.Harness) {
	fakeRoot := fsx_testkit.FakeRoot().JoinComponents("find-executable", "preconditions")

	h.Run("Empty name panics", func(h check.Harness) {
		h.Parallel()

		fs := newMemFS(h, fakeRoot.JoinComponents("empty-name"))
		want := assert.AssertionError{Fmt: "precondition violation: name is empty", Args: nil}
		h.AssertPanicsWith(want, func() {
			_, _ = envx_path.FindExecutable(fs, envx.Empty(), "")
		})
	})

	h.Run("Path separators panic", func(h check.Harness) {
		h.Parallel()

		fs := newMemFS(h, fakeRoot.JoinComponents("path-separators"))
		want := assert.AssertionError{
			Fmt:  "precondition violation: name contains path separators: %q",
			Args: []any{"dir/tool"},
		}
		h.AssertPanicsWith(want, func() {
			_, _ = envx_path.FindExecutable(fs, envx.Empty(), "dir/tool")
		})
	})
}

func newMemFS(h check.Harness, root AbsPath) fsx.FS {
	h.T().Helper()
	return Do(fsx.MemMap(root))(h)
}

func assertProblemCode(h check.Harness, err error, want errorx.Code) {
	h.T().Helper()
	problem, ok := errorx.GetRootCauseAs[*errorx.Problem](err).Get()
	h.Assertf(ok, "expected *errorx.Problem, got %T", err)
	check.AssertSame(h, want, problem.Code(), "Problem.Code()")
}

func testEnv(pairs map[string]string) envx.Env {
	return envx.New(iterx.FromMap(pairs))
}

type hostExecutable struct {
	Name       string
	LookupName string
}

func hostOSExecutable() hostExecutable {
	if runtime.GOOS == "windows" {
		return hostExecutable{Name: "tool.exe", LookupName: "tool"}
	}
	return hostExecutable{Name: "tool", LookupName: "tool"}
}

func writeExecutable(h check.Harness, fs fsx.FS, path RelPath) {
	h.T().Helper()
	h.NoErrorf(fs.WriteFile(path, []byte("#!/bin/sh\n"), 0o755), "WriteFile(%q)", path)
}
