// Copyright 2026 Varun Gandhi
//
// SPDX-License-Identifier: PolyForm-Noncommercial-1.0.0

package option_test

import (
	"testing"

	"github.com/typesanitizer/happygo/common/assert"
	"github.com/typesanitizer/happygo/common/check"
	. "github.com/typesanitizer/happygo/common/core/option"
)

func TestOption(t *testing.T) {
	h := check.New(t)
	h.Parallel()

	h.Run("Some", func(h check.Harness) {
		h.Parallel()

		opt := Some(42)
		h.Assertf(opt.IsSome(), "IsSome after Some(..)")
		h.Assertf(!opt.IsNone(), "Some => !None")
		v, ok := opt.Get()
		h.Assertf(ok && v == 42, "Get() = (%d, %v), want (42, true)", v, ok)
	})

	h.Run("None", func(h check.Harness) {
		h.Parallel()

		opt := None[int]()
		h.Assertf(opt.IsNone(), "None() => IsNone")
		h.Assertf(!opt.IsSome(), "None => !IsSome")
		_, ok := opt.Get()
		h.Assertf(!ok, "Get() on None should return false")
	})

	h.Run("Unwrap", func(h check.Harness) {
		h.Parallel()

		h.Assertf(Some(42).Unwrap() == 42, "Some(42).Unwrap() = %d, want 42", Some(42).Unwrap())
		want := assert.AssertionError{Fmt: "precondition violation: called Unwrap on None", Args: nil}
		h.AssertPanicsWith(want, func() {
			_ = None[int]().Unwrap()
		})
	})

	h.Run("ValueOr", func(h check.Harness) {
		h.Parallel()

		some := Some(10)
		h.Assertf(some.ValueOr(99) == 10, "Some(10).ValueOr(99) = %d, want 10", some.ValueOr(99))
		none := None[int]()
		h.Assertf(none.ValueOr(99) == 99, "None().ValueOr(99) = %d, want 99", none.ValueOr(99))
	})

	h.Run("Compare", func(h check.Harness) {
		h.Parallel()

		// Both Some: delegates to cmp.Compare on inner values.
		h.Assertf(Compare(Some(1), Some(2)) < 0, "Some(1) < Some(2)")
		h.Assertf(Compare(Some(2), Some(2)) == 0, "Some(2) == Some(2)")
		h.Assertf(Compare(Some(3), Some(2)) > 0, "Some(3) > Some(2)")

		// Both None: equal.
		h.Assertf(Compare(None[int](), None[int]()) == 0, "None == None")

		// None < Some (absent values sort before present).
		h.Assertf(Compare(None[int](), Some(0)) < 0, "None < Some(0)")
		h.Assertf(Compare(Some(0), None[int]()) > 0, "Some(0) > None")
	})

	h.Run("NewOption", func(h check.Harness) {
		h.Parallel()

		some := NewOption("hello", true)
		h.Assertf(some.IsSome(), "NewOption with ok=true should be Some")
		none := NewOption("hello", false)
		h.Assertf(none.IsNone(), "NewOption with ok=false should be None")
	})
}
