package collections

import (
	"testing"

	"github.com/typesanitizer/happygo/common/assert"
	"github.com/typesanitizer/happygo/common/check"
)

func TestInsert(t *testing.T) {
	h := check.New(t)
	h.Parallel()

	s := NewSet[string]()
	h.Assertf(s.Insert("a").AsBool(), "first insert should report a new value")
	h.Assertf(!s.Insert("a").AsBool(), "duplicate insert should report an existing value")
}

func TestInsertNew(t *testing.T) {
	h := check.New(t)
	h.Parallel()

	s := NewSet[string]()
	s.InsertNew("a")
	want := assert.AssertionError{Fmt: "precondition violation: set already contains value %v", Args: []any{"a"}}
	h.AssertPanicsWith(want, func() {
		s.InsertNew("a")
	})
}

func TestContains(t *testing.T) {
	h := check.New(t)
	h.Parallel()

	s := NewSet[int]()
	s.InsertNew(1)
	h.Assertf(s.Contains(1), "set should contain inserted value")
	h.Assertf(!s.Contains(2), "set should not contain non-inserted value")
}

func TestValuesNonDet(t *testing.T) {
	h := check.New(t)
	h.Parallel()

	s := NewSet[int]()
	s.InsertNew(1)
	s.InsertNew(2)
	s.InsertNew(3)

	seen := map[int]bool{}
	for v := range s.ValuesNonDet() {
		seen[v] = true
	}
	h.Assertf(len(seen) == 3, "expected 3 values, got %d", len(seen))
	for _, v := range []int{1, 2, 3} {
		h.Assertf(seen[v], "missing value %d", v)
	}
}
