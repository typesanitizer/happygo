// Copyright 2026 Varun Gandhi
//
// SPDX-License-Identifier: PolyForm-Noncommercial-1.0.0

package errorx

import (
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/typesanitizer/happygo/common/assert"
	"github.com/typesanitizer/happygo/common/check"
)

//nolint:exhaustruct // concise test expectations for RootCauseResult are clearer here
func TestGetRootCause(t *testing.T) {
	rootCauseResultCmpOpts := []cmp.Option{
		cmp.AllowUnexported(RootCauseResult{}),
		cmp.Comparer(func(x, y error) bool {
			return x == y
		}),
	}

	h := check.New(t)
	h.Parallel()

	h.Run("basic tests", func(h check.Harness) {
		sentinel := New("nostack", "sentinel")

		leaf1 := &errorNode{val: 1, inner: nil}
		leaf2 := &errorNode{val: 2, inner: nil}

		fork12 := &errorNode{val: 12, inner: []error{leaf1, leaf2}}
		twoSentinels := &errorNode{val: 99, inner: []error{sentinel, sentinel}}
		wrappedLeaf1 := Wrapf("nostack", leaf1, "wrapping")
		causedLeaf1 := &errorWithCause{inner: leaf1}

		wrappedEmpty := &errorWithUnwrap{inner: nil}
		causedEmpty := &errorWithCause{inner: nil}

		chain12 := &errorNode{val: 1, inner: []error{leaf2}}

		type TestCase struct {
			name     string
			input    error
			expected RootCauseResult
		}

		testCases := []TestCase{
			{
				name:     "sentinel",
				input:    sentinel,
				expected: RootCauseResult{err: sentinel, kind: linkNormal},
			},
			{
				name:     "simple leaf",
				input:    leaf1,
				expected: RootCauseResult{err: leaf1, kind: linkNormal},
			},
			{
				name:     "hitting multi error",
				input:    fork12,
				expected: RootCauseResult{err: fork12, kind: linkMultiError},
			},
			{
				name:     "no notion of equality checking/coalescing on hitting multi-errors",
				input:    twoSentinels,
				expected: RootCauseResult{err: twoSentinels, kind: linkMultiError},
			},
			{
				name:     "root cause found in chain",
				input:    chain12,
				expected: RootCauseResult{err: leaf2, kind: linkNormal},
			},
			{
				name:     "root cause found after wrapping (via Unwrap())",
				input:    wrappedLeaf1,
				expected: RootCauseResult{err: leaf1, kind: linkNormal},
			},
			{
				name:     "root cause found after wrapping (via Cause())",
				input:    causedLeaf1,
				expected: RootCauseResult{err: leaf1, kind: linkNormal},
			},
			{
				name:     "root cause is prior if Unwrap() == nil",
				input:    wrappedEmpty,
				expected: RootCauseResult{err: wrappedEmpty, kind: linkNormal},
			},
			{
				name:     "root cause is prior if Cause() == nil",
				input:    causedEmpty,
				expected: RootCauseResult{err: causedEmpty, kind: linkNormal},
			},
		}

		for _, tc := range testCases {
			h.Run(tc.name, func(h check.Harness) {
				h.Parallel()

				got := GetRootCause(tc.input)
				check.AssertSame(h, tc.expected, got, "root cause result", rootCauseResultCmpOpts...)
			})
		}
	})

	h.Run("nesting limit", func(h check.Harness) {
		h.Parallel()

		got := GetRootCause(makeTooDeepError())
		h.Assertf(got.HitNestingLimit(), "errors nested %d times; should exceed limit %d", NESTING_LIMIT+1, NESTING_LIMIT)
	})

	h.Run("cycle hits nesting limit", func(h check.Harness) {
		h.Parallel()

		cycle := &errorNode{val: 1, inner: nil}
		cycle.inner = []error{cycle}

		got := GetRootCause(cycle)
		h.Assertf(got.HitNestingLimit(), "cycle should hit nesting limit")
	})
}

func TestGetRootCauseAs(t *testing.T) {
	h := check.New(t)
	h.Parallel()

	h.Run("basic tests", func(h check.Harness) {
		h.Parallel()

		leaf := &errorNode{val: 1, inner: nil}
		wrapped := &errorWithUnwrap{inner: leaf}
		fork := &errorNode{val: 20, inner: []error{leaf, New("nostack", "other")}}

		gotLeaf, ok := GetRootCauseAs[*errorNode](wrapped).Get()
		h.Assertf(ok, "should find a matching root cause")
		h.Assertf(gotLeaf == leaf, "returned the wrong root cause")

		_, ok = GetRootCauseAs[*errorWithUnwrap](wrapped).Get()
		h.Assertf(!ok, "should return None for a mismatched root type")

		_, ok = GetRootCauseAs[*errorNode](fork).Get()
		h.Assertf(!ok, "should return None on multi-error")
	})

	h.Run("panic tests", func(h check.Harness) {
		h.Parallel()

		want := assert.AssertionError{Fmt: "precondition violation: %s", Args: []any{"hit nesting limit during traversal; cannot cast root cause"}}
		h.AssertPanicsWith(want, func() {
			_ = GetRootCauseAs[*errorNode](makeTooDeepError())
		})
	})
}

func TestGetRootCauseAsValue(t *testing.T) {
	h := check.New(t)
	h.Parallel()

	h.Run("basic tests", func(h check.Harness) {
		h.Parallel()

		leaf := &errorNode{val: 1, inner: nil}
		otherLeaf := &errorNode{val: 1, inner: nil}
		fork := &errorNode{val: 20, inner: []error{leaf, otherLeaf}}

		h.Assertf(GetRootCauseAsValue(leaf, leaf), "should match the exact root cause value")
		h.Assertf(!GetRootCauseAsValue(leaf, otherLeaf), "should reject a different comparable value")
		h.Assertf(!GetRootCauseAsValue(fork, leaf), "should return false on multi-error")

		matching := errorWithIs{tag: "match", xs: []int{1}}
		matchingRef := errorWithIs{tag: "match", xs: []int{2}}
		nonMatchingRef := errorWithIs{tag: "other", xs: []int{1}}
		h.Assertf(GetRootCauseAsValue(matching, matchingRef), "should consult Is(error) when available")
		h.Assertf(!GetRootCauseAsValue(matching, nonMatchingRef), "should return false when Is(error) says no")
	})

	h.Run("panic tests", func(h check.Harness) {
		h.Parallel()

		h.AssertPanicsWith(assert.AssertionError{Fmt: "precondition violation: %s", Args: []any{"expected non-nil reference error"}}, func() {
			_ = GetRootCauseAsValue(New("nostack", "sentinel"), nil)
		})
		h.AssertPanicsWith(assert.AssertionError{Fmt: "precondition violation: %s", Args: []any{"hit nesting limit during traversal; cannot cast root cause"}}, func() {
			_ = GetRootCauseAsValue(makeTooDeepError(), New("nostack", "sentinel"))
		})
	})
}

type errorNode struct {
	val   int
	inner []error
}

func (e *errorNode) Error() string {
	if len(e.inner) == 0 {
		return fmt.Sprintf("Err{%d}", e.val)
	}
	return fmt.Sprintf("Err{val: %d, inner: %v}", e.val, e.inner)
}

func (e *errorNode) Unwrap() []error {
	return e.inner
}

type errorWithUnwrap struct {
	inner error
}

func (e *errorWithUnwrap) Error() string {
	if e.inner == nil {
		return "errorWithUnwrap"
	}
	return "errorWithUnwrap: " + e.inner.Error()
}

func (e *errorWithUnwrap) Unwrap() error {
	return e.inner
}

type errorWithCause struct {
	inner error
}

func (e *errorWithCause) Error() string {
	if e.inner == nil {
		return "errorWithCause"
	}
	return "errorWithCause: " + e.inner.Error()
}

func (e *errorWithCause) Cause() error {
	return e.inner
}

type errorWithIs struct {
	tag string
	xs  []int
}

func (e errorWithIs) Error() string {
	return fmt.Sprintf("errorWithIs(%s)", e.tag)
}

func (e errorWithIs) Is(target error) bool {
	t, ok := target.(errorWithIs)
	return ok && e.tag == t.tag
}

func makeTooDeepError() error {
	base := New("nostack", "sentinel")
	var err error
	for i := range NESTING_LIMIT {
		err = &errorNode{val: i + 1, inner: []error{base}}
		base = err
	}
	return err
}
