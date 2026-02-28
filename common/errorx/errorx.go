// Package errorx wraps cockroachdb/errors for consistent error handling.
// All error creation in first-party code should go through this package.
package errorx

import (
	"errors" //nolint:depguard // In designated wrapper package
	"fmt"

	cockroach "github.com/cockroachdb/errors" //nolint:depguard // In designated wrapper package
	"github.com/typesanitizer/happygo/common/assert"
)

// IncludeStackTrace controls whether a stack trace is captured when creating an error.
// Callers deliberately use the raw string literals "+stacks" / "nostack" for
// readability at call sites; these constants exist for documentation purposes.
type IncludeStackTrace string

const (
	IncludeStackTraceCapture IncludeStackTrace = "+stacks"
	IncludeStackTraceSkip    IncludeStackTrace = "nostack"
)

func New(ist IncludeStackTrace, msg string) error {
	switch ist {
	case IncludeStackTraceSkip:
		return errors.New(msg) //nolint:forbidigo // errorx is the designated wrapper
	case IncludeStackTraceCapture:
		return cockroach.NewWithDepth(1, msg)
	default:
		return assert.PanicUnknownCase[error](ist)
	}
}

func Newf(ist IncludeStackTrace, format string, args ...any) error {
	switch ist {
	case IncludeStackTraceSkip:
		return fmt.Errorf(format, args...) //nolint:forbidigo // errorx is the designated wrapper
	case IncludeStackTraceCapture:
		return cockroach.NewWithDepthf(1, format, args...)
	default:
		return assert.PanicUnknownCase[error](ist)
	}
}

// Append combines multiple errors into one, filtering out nils.
func Join(errs ...error) error {
	return cockroach.Join(errs...)
}

func Wrapf(ist IncludeStackTrace, err error, format string, args ...any) error {
	switch ist {
	case IncludeStackTraceSkip:
		return cockroach.WithMessagef(err, format, args...)
	case IncludeStackTraceCapture:
		return cockroach.WrapWithDepthf(1, err, format, args...)
	default:
		return assert.PanicUnknownCase[error](ist)
	}
}
