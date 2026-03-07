package collections

import (
	"testing"

	"github.com/typesanitizer/happygo/common/check"
)

func TestInsert(t *testing.T) {
	t.Parallel()
	h := check.New(t)

	s := NewSet[string]()
	h.Assertf(s.Insert("a"), "first insert should return true")
	h.Assertf(!s.Insert("a"), "duplicate insert should return false")
}

func TestContains(t *testing.T) {
	t.Parallel()
	h := check.New(t)

	s := NewSet[int]()
	s.Insert(1)
	h.Assertf(s.Contains(1), "set should contain inserted value")
	h.Assertf(!s.Contains(2), "set should not contain non-inserted value")
}

func TestValuesNonDet(t *testing.T) {
	t.Parallel()
	h := check.New(t)

	s := NewSet[int]()
	s.Insert(1)
	s.Insert(2)
	s.Insert(3)

	seen := map[int]bool{}
	for v := range s.ValuesNonDet() {
		seen[v] = true
	}
	h.Assertf(len(seen) == 3, "expected 3 values, got %d", len(seen))
	for _, v := range []int{1, 2, 3} {
		h.Assertf(seen[v], "missing value %d", v)
	}
}
