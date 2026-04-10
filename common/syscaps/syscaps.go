// Package syscaps provides controlled access to ambient system capabilities.
package syscaps

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/afero" //nolint:depguard // syscaps is the ambient-authority boundary

	"github.com/typesanitizer/happygo/common/assert"
	"github.com/typesanitizer/happygo/common/cmdx"
	. "github.com/typesanitizer/happygo/common/core"
	"github.com/typesanitizer/happygo/common/core/pair"
	"github.com/typesanitizer/happygo/common/envx"
	"github.com/typesanitizer/happygo/common/errorx"
	"github.com/typesanitizer/happygo/common/fsx"
	"github.com/typesanitizer/happygo/common/logx"
)

// Env returns the current process environment.
func Env() envx.Env {
	return envx.NewIgnoringDupes(func(yield func(pair.KeyValue[string, string]) bool) {
		for _, entry := range os.Environ() { //nolint:forbidigo // syscaps is the ambient-authority boundary
			key, value, ok := strings.Cut(entry, "=")
			assert.Postconditionf(ok, "os.Environ entry missing '=': %q", entry)
			if !yield(pair.NewKeyValue(key, value)) {
				return
			}
		}
	})
}

// FS returns a rooted filesystem backed by the host operating system.
func FS(root AbsPath) (fsx.FS, error) {
	return fsx.NewRootedFS(root, afero.NewOsFs())
}

// CmdRunner executes commands using ambient system capabilities.
type CmdRunner struct {
	Env envx.Env
}

func (runner CmdRunner) Run(ctx logx.LogCtx, cmd cmdx.Cmd, options cmdx.RunOptions) (string, error) {
	dir, hasDir := cmd.Dir().Get()
	if hasDir {
		ctx.Debug("running command", "cmd", cmd, "dir", dir.String())
	} else {
		ctx.Debug("running command", "cmd", cmd)
	}

	stdout, stderr := ctx.CmdLoggers(cmd)
	defer logx.FlushLogWriter(stdout)
	defer logx.FlushLogWriter(stderr)

	argv := cmd.Argv()
	execCmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	if hasDir {
		execCmd.Dir = dir.String()
	}
	if options.TransformEnv != nil {
		execCmd.Env = options.TransformEnv(runner.Env).Entries()
	} else {
		execCmd.Env = runner.Env.Entries()
	}

	var capturedOutput bytes.Buffer
	if options.CaptureStdout {
		execCmd.Stdout = io.MultiWriter(stdout, &capturedOutput)
	} else {
		execCmd.Stdout = stdout
	}
	execCmd.Stderr = stderr

	if err := execCmd.Run(); err != nil {
		return capturedOutput.String(), errorx.Wrapf("+stacks", err, "%s", cmd)
	}
	return capturedOutput.String(), nil
}

func (runner CmdRunner) ExecAll(ctx logx.LogCtx, cmds ...cmdx.Cmd) error {
	return cmdx.BaseRunnerExecAll(runner, ctx, cmds...)
}
