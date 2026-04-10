package collections

import (
	"slices"
	"testing"

	"pgregory.net/rapid"

	"github.com/typesanitizer/happygo/common/check"
	"github.com/typesanitizer/happygo/common/core/op"
)

func TestMonotoneMap(t *testing.T) {
	h := check.New(t)

	h.Run("Unit", func(h check.Harness) {
		h.Parallel()

		m := NewMonotoneMap[string, int]()
		h.Assertf(!m.Lookup("missing").IsSome(), "missing key unexpectedly present")
		h.Assertf(m.InsertOrKeep("a", 1) == op.InsertedNew, "first insert should report InsertedNew")
		h.Assertf(m.InsertOrKeep("a", 2) == op.KeptOld, "duplicate insert should report KeptOld")

		old, ok := m.InsertOrReplace("a", 3).Get()
		h.Assertf(ok, "InsertOrReplace should return the old value")
		h.Assertf(old == 1, "InsertOrReplace returned old value %d, want 1", old)

		h.Assertf(m.InsertOrKeep("b", 4) == op.InsertedNew, "insert of b should report InsertedNew")

		gotKeys := slices.Collect(m.Keys())
		wantKeys := []string{"a", "b"}
		check.AssertSame(h, wantKeys, gotKeys, "Keys()")

		omit := NewSet[string]()
		omit.InsertNew("a")
		filtered := m.CloneWithout(omit)
		gotFilteredKeys := slices.Collect(filtered.Keys())
		wantFilteredKeys := []string{"b"}
		check.AssertSame(h, wantFilteredKeys, gotFilteredKeys, "filtered Keys()")
		h.Assertf(!filtered.Lookup("a").IsSome(), "filtered map unexpectedly contains omitted key")

		gotB, ok := filtered.Lookup("b").Get()
		h.Assertf(ok, "filtered map missing key b")
		h.Assertf(gotB == 4, "filtered map returned %d for b, want 4", gotB)
	})

	h.Run("InsertOrKeepProperties", func(h check.Harness) {
		h.Parallel()

		keysGen := rapid.SliceOfN(rapid.Int(), 0, 10)
		rapid.Check(h.T(), func(t *rapid.T) {
			h := check.NewBasic(t)
			keys := keysGen.Draw(t, "keys")

			m := NewMonotoneMap[int, int]()
			for i, key := range keys {
				beforeOrder := slices.Collect(m.Keys())
				beforeLen := m.Len()
				beforeValue, hadBefore := m.Lookup(key).Get()

				res := m.InsertOrKeep(key, i)

				afterOrder := slices.Collect(m.Keys())
				got, ok := m.Lookup(key).Get()
				h.Assertf(ok, "Lookup(%d) returned None after InsertOrKeep", key)

				if !hadBefore {
					h.Assertf(res == op.InsertedNew,
						"InsertOrKeep(%d, %d) = %v, want InsertedNew", key, i, res)
					h.Assertf(m.Len() == beforeLen+1,
						"Len() = %d, want %d after inserting %d", m.Len(), beforeLen+1, key)
					expectedOrder := slices.Clone(beforeOrder)
					expectedOrder = append(expectedOrder, key)
					check.AssertSame(h, expectedOrder, afterOrder, "Keys() after inserting new key")
					h.Assertf(got == i,
						"Lookup(%d) = %d, want %d after InsertOrKeep", key, got, i)
					continue
				}

				h.Assertf(res == op.KeptOld,
					"InsertOrKeep(%d, %d) = %v, want KeptOld", key, i, res)
				h.Assertf(m.Len() == beforeLen,
					"Len() = %d, want %d after re-inserting %d", m.Len(), beforeLen, key)
				check.AssertSame(h, beforeOrder, afterOrder, "Keys() after re-inserting existing key")
				h.Assertf(got == beforeValue,
					"Lookup(%d) = %d, want preserved value %d", key, got, beforeValue)
			}
		})
	})

	h.Run("InsertOrReplaceProperties", func(h check.Harness) {
		h.Parallel()

		keysGen := rapid.SliceOfN(rapid.Int(), 0, 10)
		rapid.Check(h.T(), func(t *rapid.T) {
			h := check.NewBasic(t)
			keys := keysGen.Draw(t, "keys")

			m := NewMonotoneMap[int, int]()
			wantLatest := map[int]int{}
			for i, key := range keys {
				beforeOrder := slices.Collect(m.Keys())
				beforeLen := m.Len()
				beforeIndex := slices.Index(beforeOrder, key)

				old, hadOld := m.InsertOrReplace(key, i).Get()

				afterOrder := slices.Collect(m.Keys())
				afterIndex := slices.Index(afterOrder, key)
				got, ok := m.Lookup(key).Get()
				h.Assertf(ok, "Lookup(%d) returned None after InsertOrReplace", key)
				h.Assertf(got == i, "Lookup(%d) = %d, want %d", key, got, i)

				if beforeIndex < 0 {
					h.Assertf(!hadOld,
						"InsertOrReplace(%d, %d) unexpectedly returned old value %d", key, i, old)
					h.Assertf(m.Len() == beforeLen+1,
						"Len() = %d, want %d after inserting %d", m.Len(), beforeLen+1, key)
					expectedOrder := slices.Clone(beforeOrder)
					expectedOrder = append(expectedOrder, key)
					check.AssertSame(h, expectedOrder, afterOrder, "Keys() after inserting new key")
					h.Assertf(afterIndex == len(beforeOrder),
						"position of %d = %d, want %d", key, afterIndex, len(beforeOrder))
					wantLatest[key] = i
					continue
				}

				h.Assertf(hadOld,
					"InsertOrReplace(%d, %d) did not return previous value", key, i)
				h.Assertf(old == wantLatest[key],
					"InsertOrReplace(%d, %d) returned %d, want %d", key, i, old, wantLatest[key])
				h.Assertf(m.Len() == beforeLen,
					"Len() = %d, want %d after replacing %d", m.Len(), beforeLen, key)
				check.AssertSame(h, beforeOrder, afterOrder, "Keys() after replacing existing key")
				h.Assertf(afterIndex == beforeIndex,
					"position of %d = %d, want %d", key, afterIndex, beforeIndex)
				wantLatest[key] = i
			}
		})
	})

	h.Run("CloneWithoutProperties", func(h check.Harness) {
		h.Parallel()

		keysGen := rapid.SliceOfN(rapid.Int(), 0, 10)
		omitKeysGen := rapid.SliceOfN(rapid.Int(), 0, 10)
		rapid.Check(h.T(), func(t *rapid.T) {
			h := check.NewBasic(t)
			keys := keysGen.Draw(t, "keys")
			omitKeys := omitKeysGen.Draw(t, "omit_keys")

			m := NewMonotoneMap[int, int]()
			wantOrder := make([]int, 0)
			wantLatest := map[int]int{}
			seen := map[int]struct{}{}
			for i, key := range keys {
				if _, ok := seen[key]; !ok {
					wantOrder = append(wantOrder, key)
					seen[key] = struct{}{}
				}
				_ = m.InsertOrReplace(key, i)
				wantLatest[key] = i
			}

			omit := NewSet[int]()
			for _, key := range omitKeys {
				omit.Insert(key)
			}

			filtered := m.CloneWithout(omit)
			var wantFilteredOrder []int
			removedCount := 0
			for _, key := range wantOrder {
				if omit.Contains(key) {
					removedCount++
					continue
				}
				wantFilteredOrder = append(wantFilteredOrder, key)
			}

			gotFilteredOrder := slices.Collect(filtered.Keys())
			check.AssertSame(h, wantFilteredOrder, gotFilteredOrder, "filtered Keys()")
			h.Assertf(filtered.Len() == m.Len()-removedCount,
				"filtered Len() = %d, want %d", filtered.Len(), m.Len()-removedCount)
			for _, key := range wantOrder {
				got, ok := filtered.Lookup(key).Get()
				if omit.Contains(key) {
					h.Assertf(!ok, "filtered Lookup(%d) unexpectedly returned %d", key, got)
					continue
				}
				h.Assertf(ok, "filtered Lookup(%d) returned None", key)
				h.Assertf(got == wantLatest[key],
					"filtered Lookup(%d) = %d, want %d", key, got, wantLatest[key])
			}
		})
	})
}
