// Copyright 2026 Varun Gandhi
//
// SPDX-License-Identifier: PolyForm-Noncommercial-1.0.0

package result_test

import (
	"testing"

	"github.com/typesanitizer/happygo/common/assert"
	"github.com/typesanitizer/happygo/common/check"
	. "github.com/typesanitizer/happygo/common/core/result"
	"github.com/typesanitizer/happygo/common/errorx"
)

func TestResult(t *testing.T) {
	h := check.New(t)
	h.Parallel()

	h.Run("Success", func(h check.Harness) {
		h.Parallel()

		r := Success(42)
		h.Assertf(r.ErrOrNil() == nil, "Success.ErrOrNil() should be nil")
		v, err := r.Get()
		h.Assertf(v == 42 && err == nil, "Get() = (%d, %v), want (42, nil)", v, err)
	})

	h.Run("Failure", func(h check.Harness) {
		h.Parallel()

		want := errorx.New("nostack", "boom")
		r := Failure[int](want)
		h.Assertf(r.ErrOrNil() == want, "Failure.ErrOrNil() = %v, want %v", r.ErrOrNil(), want)
		v, err := r.Get()
		h.Assertf(v == 0 && err == want, "Get() = (%d, %v), want (0, %v)", v, err, want)
	})

	h.Run("FailureNilPanics", func(h check.Harness) {
		h.Parallel()

		wantPanic := assert.AssertionError{
			Fmt:  "precondition violation: Failure called with nil error",
			Args: nil,
		}
		h.AssertPanicsWith(wantPanic, func() {
			_ = Failure[int](nil)
		})
	})
}
