// Package errorx wraps cockroachdb/errors for consistent error handling.
// All error creation in first-party code should go through this package.
package errorx

import (
	"errors" //nolint:depguard // In designated wrapper package
	"fmt"

	cockroach "github.com/cockroachdb/errors" //nolint:depguard // In designated wrapper package
	"github.com/typesanitizer/happygo/common/assert"
	. "github.com/typesanitizer/happygo/common/core"
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

const NESTING_LIMIT = 1000

// RootCauseResult represents one of two cases:
//
//  1. Root cause: A root cause was found by following a linear chain
//     from the original error (this may be the original error itself).
//  2. Multi-error: When traversing the error tree, a multi-error with
//     2 or more sub-errors was hit. In this case, it is not generally
//     correct to mark one of the errors as the root cause.
//  3. Hit nesting limit (~rare): Traversal hit [NESTING_LIMIT] iterations.
//     This generally indicates a bug somewhere.
//
// The case can be checked using HitNestingLimit() and HitMultiError().
type RootCauseResult struct {
	// Logically, there are two cases.
	// 1. We hit a multi-error with 2+ causes when traversing the error tree.
	//    If this is the case, hitMultiError will be set to non-nil with that
	//    multi-error.
	// 2. We didn't hit a multi-error. In this case rootCause will be set to
	//    a non-nil error.
	rootCause       error
	hitMultiError   error
	hitNestingLimit bool
}

func (r RootCauseResult) HitNestingLimit() bool {
	return r.hitNestingLimit
}

// HitMultiError returns whether an error tree traversal hit a multi-error with
// 2 or more sub-errors.
func (r RootCauseResult) HitMultiError() bool {
	return !r.hitNestingLimit && r.hitMultiError != nil
}

// GetMultiError returns the first multi-error found during error tree traversal.
//
// Pre-condition: This result must be a multi-error.
func (r RootCauseResult) GetMultiError() error {
	assert.Preconditionf(!r.hitNestingLimit, "requesting multi-error but hit nesting limit %v during traversal", NESTING_LIMIT)
	assert.Preconditionf(r.hitMultiError != nil, "requesting multi-error but found root cause: %v", r.rootCause)
	return r.hitMultiError
}

// GetRootCause returns a non-nil root cause.
//
// Pre-condition: This result must not be a multi-error.
func (r RootCauseResult) GetRootCause() error {
	assert.Preconditionf(!r.hitNestingLimit, "requesting root cause but hit nesting limit %v during traversal", NESTING_LIMIT)
	assert.Preconditionf(!r.HitMultiError(), "requesting root cause despite hitting multi-error: %v", r.hitMultiError)
	return r.rootCause
}

// GetRootCause traverses the provided error tree, and gets the underlying
// root cause, if one can be found by following a chain of wrapper errors.
//
// If there is a "fork" in the tree (i.e. a multi-error with 2 or more
// sub-errors is found), then that multi-error is returned instead.
//
// The tree traversal is based on the following methods:
//   - Unwrap() []error
//   - Unwrap() error
//   - Cause() error
//
// Pre-condition: err != nil
func GetRootCause(err error) RootCauseResult {
	assert.Precondition(err != nil, "trying to get root cause for nil error")
	cur := err
	for range NESTING_LIMIT {
		switch e := cur.(type) {
		case interface{ Unwrap() []error }:
			errList := e.Unwrap()
			switch len(errList) {
			case 0:
				return RootCauseResult{rootCause: cur, hitMultiError: nil, hitNestingLimit: false}
			case 1:
				cur = errList[0]
				continue
			default:
				return RootCauseResult{rootCause: nil, hitMultiError: cur, hitNestingLimit: false}
			}
		case interface{ Unwrap() error }:
			inner := e.Unwrap()
			if inner != nil {
				cur = inner
				continue
			}
			return RootCauseResult{rootCause: cur, hitMultiError: nil, hitNestingLimit: false}
		case interface{ Cause() error }:
			inner := e.Cause()
			if inner != nil {
				cur = inner
				continue
			}
			return RootCauseResult{rootCause: cur, hitMultiError: nil, hitNestingLimit: false}
		default:
			return RootCauseResult{rootCause: cur, hitMultiError: nil, hitNestingLimit: false}
		}
	}
	return RootCauseResult{rootCause: nil, hitMultiError: nil, hitNestingLimit: true}
}

// GetRootCauseAs is similar to GetRootCause except that it tries to cast
// the root cause (if one was found) to the type parameter.
//
// Pre-condition: err != nil, and err's nesting does not exceed [NESTING_LIMIT].
func GetRootCauseAs[T error](err error) Option[T] {
	r := GetRootCause(err)
	assert.Precondition(!r.HitNestingLimit(), "hit nesting limit during traversal; cannot cast root cause")
	if r.HitMultiError() {
		return None[T]()
	}
	v, ok := r.GetRootCause().(T)
	return NewOption(v, ok)
}

// GetRootCauseAsValue is similar to GetRootCause except that it tries to compare
// the root cause (if one was found) to the reference error value.
//
// Comparison is done with Is(error) bool on err, if available.
// If Is(error) bool is unavailable, comparison is done using ==.
//
// Returns false if a multi-error was hit, or if the comparison fails.
//
// Pre-conditions: err != nil, reference != nil, and err's nesting does not exceed [NESTING_LIMIT].
//
// CAUTION: reference should generally not have Error() with a value receiver,
// unless it is guaranteed to be comparable. Otherwise, it's possible for
// comparison to == to panic (e.g. if the type contains slices).
func GetRootCauseAsValue(err error, reference error) bool {
	assert.Precondition(reference != nil, "expected non-nil reference error")
	r := GetRootCause(err)
	assert.Precondition(!r.HitNestingLimit(), "hit nesting limit during traversal; cannot cast root cause")
	if r.HitMultiError() {
		return false
	}
	rc := r.GetRootCause()
	switch rct := rc.(type) {
	case interface{ Is(error) bool }:
		return rct.Is(reference)
	default:
		return rc == reference
	}
}
