package gdbserial

import (
	"io"
	"os"

	"github.com/go-delve/delve/pkg/logflags"
)

// LogConfig controls gdbserial logging output.
type LogConfig struct {
	// GdbWire enables gdb remote protocol packet logging.
	GdbWire bool
	// GdbWireOut is the destination for gdb wire logs. Nil uses the global log destination.
	GdbWireOut io.Writer

	// LLDBServerOutput enables verbose debugserver output (-g -l stdout).
	LLDBServerOutput bool
	// LLDBServerOut is the destination for debugserver stdout when enabled.
	LLDBServerOut io.Writer
	// LLDBServerErr is the destination for debugserver stderr when enabled.
	LLDBServerErr io.Writer

	// ProcessLifecycle enables logging around gdbserial process lifecycle.
	ProcessLifecycle bool
	// ProcessLifecycleOut is the destination for lifecycle logs. Nil uses the global log destination.
	ProcessLifecycleOut io.Writer
}

// ProcessConfig configures gdbserial process creation.
type ProcessConfig struct {
	Log LogConfig
}

// DefaultProcessConfig returns a config derived from the global logflags.
func DefaultProcessConfig() ProcessConfig {
	return ProcessConfig{
		Log: LogConfig{
			GdbWire:             logflags.GdbWire(),
			LLDBServerOutput:    logflags.LLDBServerOutput(),
			ProcessLifecycle:    false,
			GdbWireOut:          nil,
			LLDBServerOut:       nil,
			LLDBServerErr:       nil,
			ProcessLifecycleOut: nil,
		},
	}
}

func normalizeProcessConfig(cfg *ProcessConfig) ProcessConfig {
	if cfg == nil {
		return DefaultProcessConfig()
	}
	return *cfg
}

func (cfg LogConfig) gdbWireLogger() logflags.Logger {
	return logflags.NewLogger(cfg.GdbWire, logflags.Fields{"layer": "gdbconn"}, cfg.GdbWireOut)
}

func (cfg LogConfig) gdbWireEnabled() bool {
	return cfg.GdbWire
}

func (cfg LogConfig) processLifecycleLogger() logflags.Logger {
	return logflags.NewLogger(cfg.ProcessLifecycle, logflags.Fields{"layer": "gdbserial", "kind": "process"}, cfg.ProcessLifecycleOut)
}

func (cfg LogConfig) processLifecycleEnabled() bool {
	return cfg.ProcessLifecycle
}

func (cfg LogConfig) wantsServerOutput() bool {
	return cfg.LLDBServerOutput || cfg.GdbWire
}

func (cfg LogConfig) serverWriters() (io.Writer, io.Writer) {
	out := cfg.LLDBServerOut
	errOut := cfg.LLDBServerErr
	if out == nil {
		out = os.Stdout
	}
	if errOut == nil {
		errOut = os.Stderr
	}
	return out, errOut
}
