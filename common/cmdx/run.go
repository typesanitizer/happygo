package cmdx

import (
	"github.com/typesanitizer/happygo/common/envx"
	"github.com/typesanitizer/happygo/common/logx"
)

// RunOptions configures BaseRunner.Run behavior.
type RunOptions struct {
	CaptureStdout bool
	TransformEnv  func(envx.Env) envx.Env
}

// RunOptionsDefault returns default options for BaseRunner.Run.
func RunOptionsDefault() RunOptions {
	return RunOptions{CaptureStdout: false, TransformEnv: nil}
}

// WithCaptureStdout returns a copy with CaptureStdout set.
func (o RunOptions) WithCaptureStdout() RunOptions {
	o.CaptureStdout = true
	return o
}

// BaseRunner executes a single command.
//
// Run is intended for non-streaming use cases. If we later need streaming
// capture, we can add a lower-level API and implement Run on top of it.
type BaseRunner interface {
	// Run runs a command.
	//
	// The first return value is the captured stdout. There may be stdout
	// even in the presence of errors. Stdout is only captured if
	// options.CaptureStdout is true.
	Run(_ logx.LogCtx, _ Cmd, options RunOptions) (string, error)
}

// Runner executes single commands and sequential command lists.
type Runner interface {
	BaseRunner
	// ExecAll runs cmds sequentially with default options, stopping at
	// the first error.
	ExecAll(_ logx.LogCtx, cmds ...Cmd) error
}

func BaseRunnerExecAll(runner BaseRunner, ctx logx.LogCtx, cmds ...Cmd) error {
	for _, cmd := range cmds {
		if _, err := runner.Run(ctx, cmd, RunOptionsDefault()); err != nil {
			return err
		}
	}
	return nil
}
