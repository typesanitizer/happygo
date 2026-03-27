package cmdx

import (
	"bytes"
	"io"
	"os"
	"os/exec"

	"github.com/typesanitizer/happygo/common/errorx"
	"github.com/typesanitizer/happygo/common/logx"
)

// RunOptions configures Cmd.Run behavior.
type RunOptions struct {
	CaptureStdout bool
	TransformEnv  func([]string) []string
}

// RunOptionsDefault returns default options for Cmd.Run.
func RunOptionsDefault() RunOptions {
	return RunOptions{CaptureStdout: false, TransformEnv: nil}
}

// WithCaptureStdout returns a copy with CaptureStdout set.
func (o RunOptions) WithCaptureStdout() RunOptions {
	o.CaptureStdout = true
	return o
}

func (cmd Cmd) Run(ctx logx.LogCtx, options RunOptions) (string, error) {
	dir, hasDir := cmd.dir.Get()
	if hasDir {
		ctx.Debug("running command", "cmd", cmd, "dir", dir.String())
	} else {
		ctx.Debug("running command", "cmd", cmd)
	}

	stdout, stderr := ctx.CmdLoggers(cmd)
	defer logx.FlushLogWriter(stdout)
	defer logx.FlushLogWriter(stderr)

	execCmd := exec.CommandContext(ctx, cmd.name, cmd.args...)
	if hasDir {
		execCmd.Dir = dir.String()
	}
	if options.TransformEnv != nil {
		execCmd.Env = options.TransformEnv(os.Environ())
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

// ExecAll runs each command sequentially with default options, stopping at the first error.
func ExecAll(ctx logx.LogCtx, cmds ...Cmd) error {
	for _, cmd := range cmds {
		if _, err := cmd.Run(ctx, RunOptionsDefault()); err != nil {
			return err
		}
	}
	return nil
}
