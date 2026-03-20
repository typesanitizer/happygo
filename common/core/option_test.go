package core_test

import (
	"testing"

	"github.com/typesanitizer/happygo/common/check"
	. "github.com/typesanitizer/happygo/common/core"
)

func TestOption(t *testing.T) {
	h := check.New(t)
	h.Parallel()

	h.Run("Some", func(h check.Harness) {
		h.Parallel()

		opt := Some(42)
		h.Assertf(opt.IsSome(), "Some should be present")
		v, ok := opt.Get()
		h.Assertf(ok && v == 42, "Get() = (%d, %v), want (42, true)", v, ok)
	})

	h.Run("None", func(h check.Harness) {
		h.Parallel()

		opt := None[int]()
		h.Assertf(!opt.IsSome(), "None should not be present")
		_, ok := opt.Get()
		h.Assertf(!ok, "Get() on None should return false")
	})

	h.Run("ValueOr", func(h check.Harness) {
		h.Parallel()

		some := Some(10)
		h.Assertf(some.ValueOr(99) == 10, "Some(10).ValueOr(99) = %d, want 10", some.ValueOr(99))
		none := None[int]()
		h.Assertf(none.ValueOr(99) == 99, "None().ValueOr(99) = %d, want 99", none.ValueOr(99))
	})

	h.Run("NewOption", func(h check.Harness) {
		h.Parallel()

		some := NewOption("hello", true)
		h.Assertf(some.IsSome(), "NewOption with ok=true should be Some")
		none := NewOption("hello", false)
		h.Assertf(!none.IsSome(), "NewOption with ok=false should be None")
	})
}
