// Copyright 2026 Varun Gandhi
//
// SPDX-License-Identifier: PolyForm-Noncommercial-1.0.0

package logx

import (
	"context"

	"github.com/typesanitizer/happygo/common/assert"
)

// LogCtx carries a logger and context together.
type LogCtx struct {
	context.Context
	// Always non-nil.
	*Logger
}

// NewLogCtx constructs a LogCtx from context and logger.
func NewLogCtx(ctx context.Context, logger *Logger) LogCtx {
	assert.Precondition(logger != nil, "logger must be non-nil")
	return LogCtx{Context: ctx, Logger: logger}
}

// IsDebugEnabled reports whether debug-level logs are enabled.
func (ctx LogCtx) IsDebugEnabled() bool {
	return ctx.GetLevel() <= DebugLevel
}
