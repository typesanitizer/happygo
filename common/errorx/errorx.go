// Package errorx wraps cockroachdb/errors for consistent error handling.
// All error creation in first-party code should go through this package.
package errorx

import (
	"errors" //nolint:depguard // In designated wrapper package
	"fmt"
	"iter"

	cockroach "github.com/cockroachdb/errors" //nolint:depguard // In designated wrapper package
	"github.com/typesanitizer/happygo/common/assert"
	. "github.com/typesanitizer/happygo/common/core"
	"github.com/typesanitizer/happygo/common/iterx"
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

// Join combines multiple errors into one, filtering out nils.
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

type linkKind uint8

const (
	linkNormal       linkKind = iota // intermediate node or leaf
	linkMultiError                   // stopped at multi-error fork (2+ children)
	linkNestingLimit                 // exceeded NESTING_LIMIT
)

func (k linkKind) String() string {
	switch k {
	case linkNormal:
		return "normal"
	case linkMultiError:
		return "multi-error"
	case linkNestingLimit:
		return "nesting-limit"
	default:
		return assert.PanicUnknownCase[string](k)
	}
}

// ChainLink represents a single node encountered while traversing an error chain.
//
// At intermediate nodes, Current is the error at that level and the kind is
// [linkNormal]. The final yielded ChainLink carries the terminal condition:
//   - A leaf error (no Unwrap/Cause, or Unwrap returns nil): linkNormal
//   - A multi-error fork (Unwrap() []error with 2+ elements): linkMultiError
//   - Nesting limit exceeded: linkNestingLimit
type ChainLink struct {
	current error
	kind    linkKind
}

// Current returns the error at this point in the chain.
func (c ChainLink) Current() error {
	return c.current
}

// HitMultiError returns whether traversal stopped at a multi-error fork.
func (c ChainLink) HitMultiError() bool {
	return c.kind == linkMultiError
}

// HitNestingLimit returns whether traversal exceeded [NESTING_LIMIT].
func (c ChainLink) HitNestingLimit() bool {
	return c.kind == linkNestingLimit
}

// Chain yields each error in the unwrap chain from outermost to innermost.
//
// The chain is traversed via Unwrap() error, Unwrap() []error (single-element
// only), and Cause() error. Traversal stops at leaves, multi-error forks
// (2+ children), or after [NESTING_LIMIT] iterations.
//
// The final yielded [ChainLink] carries the terminal condition.
//
// Pre-condition: err != nil
// Post-condition: The iterator is non-empty, and the first element has Current() == err.
func Chain(err error) iter.Seq[ChainLink] {
	assert.Precondition(err != nil, "trying to traverse chain for nil error")
	return func(yield func(ChainLink) bool) {
		cur := err
		for range NESTING_LIMIT {
			kind := linkNormal
			var next error
			switch e := cur.(type) {
			case interface{ Unwrap() []error }:
				errList := e.Unwrap()
				switch len(errList) {
				case 0:
					break
				case 1:
					next = errList[0]
				default:
					kind = linkMultiError
				}
			case interface{ Unwrap() error }:
				next = e.Unwrap()
			case interface{ Cause() error }:
				next = e.Cause()
			}
			if !yield(ChainLink{current: cur, kind: kind}) {
				return
			}
			if kind != linkNormal || next == nil {
				return
			}
			cur = next
		}
		yield(ChainLink{current: cur, kind: linkNestingLimit})
	}
}

// RootCauseResult represents the outcome of traversing an error chain:
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
	// err holds either the root cause or the multi-error, depending on kind.
	// It is nil only when kind == linkNestingLimit.
	err  error
	kind linkKind
}

func (r RootCauseResult) HitNestingLimit() bool {
	return r.kind == linkNestingLimit
}

// HitMultiError returns whether an error tree traversal hit a multi-error with
// 2 or more sub-errors.
func (r RootCauseResult) HitMultiError() bool {
	return r.kind == linkMultiError
}

// GetMultiError returns the first multi-error found during error tree traversal.
//
// Pre-condition: This result must be a multi-error.
func (r RootCauseResult) GetMultiError() error {
	assert.Preconditionf(r.kind == linkMultiError, "requesting multi-error but kind is %s", r.kind)
	return r.err
}

// GetRootCause returns a non-nil root cause.
//
// Pre-condition: This result must be a root cause (not multi-error or nesting limit).
func (r RootCauseResult) GetRootCause() error {
	assert.Preconditionf(r.kind == linkNormal, "requesting root cause but kind is %s", r.kind)
	return r.err
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
	last := iterx.Last(Chain(err))
	return RootCauseResult{err: last.Current(), kind: last.kind}
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

// FindInChainAs traverses the error chain and returns the first error
// that matches the type parameter T.
//
// Returns None if no match is found or if a multi-error is hit before
// finding a match.
//
// You generally want to use [GetRootCauseAs] instead of this function,
// but this can be useful if the error you want to check potentially
// wraps some other errors.
//
// Pre-condition: err != nil, and err's nesting does not exceed [NESTING_LIMIT].
func FindInChainAs[T error](err error) Option[T] {
	return iterx.Find(Chain(err), func(link ChainLink) Option[T] {
		assert.Precondition(!link.HitNestingLimit(), "hit nesting limit during traversal")
		v, ok := link.Current().(T)
		return NewOption(v, ok)
	})
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
