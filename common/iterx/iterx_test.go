package iterx_test

import (
	"iter"
	"maps"
	"slices"
	"testing"

	"pgregory.net/rapid"

	"github.com/typesanitizer/happygo/common/assert"
	"github.com/typesanitizer/happygo/common/check"
	"github.com/typesanitizer/happygo/common/core/option"
	"github.com/typesanitizer/happygo/common/core/pair"
	"github.com/typesanitizer/happygo/common/core/result"
	"github.com/typesanitizer/happygo/common/errorx"
	"github.com/typesanitizer/happygo/common/iterx"
)

func TestEmpty(t *testing.T) {
	h := check.New(t)
	h.Parallel()

	h.Run("Unit", func(h check.Harness) {
		h.Parallel()

		called := false
		for range iterx.Empty[int]() {
			called = true
		}
		h.Assertf(!called, "Empty() should not yield any elements")
	})

	// Empty acts as a left and right identity for Chain
	h.Run("Properties", func(h check.Harness) {
		h.Parallel()

		valuesGen := rapid.SliceOfN(rapid.Int(), 0, 12)
		rapid.Check(h.T(), func(t *rapid.T) {
			basic := check.NewBasic(t)
			values := valuesGen.Draw(t, "values")

			left := iterx.Collect(iterx.Chain(iterx.Empty[int](), slices.Values(values)))
			basic.Assertf(slices.Equal(left, values),
				"Chain(Empty(), seq) = %#v, want %#v", left, values)

			right := iterx.Collect(iterx.Chain(slices.Values(values), iterx.Empty[int]()))
			basic.Assertf(slices.Equal(right, values),
				"Chain(seq, Empty()) = %#v, want %#v", right, values)
		})
	})
}

func TestCollect(t *testing.T) {
	h := check.New(t)
	h.Parallel()

	h.Run("Unit", func(h check.Harness) {
		h.Parallel()

		got := iterx.Collect(slices.Values([]int{1, 2, 3}))
		h.Assertf(slices.Equal(got, []int{1, 2, 3}), "Collect(seq) = %#v, want %#v", got, []int{1, 2, 3})

		empty := iterx.Collect(slices.Values([]int{}))
		h.Assertf(len(empty) == 0, "Collect(empty) length = %d, want 0", len(empty))
	})

	h.Run("Properties", func(h check.Harness) {
		h.Parallel()

		valuesGen := rapid.SliceOfN(rapid.Int(), 0, 12)
		rapid.Check(h.T(), func(t *rapid.T) {
			basic := check.NewBasic(t)
			values := valuesGen.Draw(t, "values")
			got := iterx.Collect(slices.Values(values))
			basic.Assertf(slices.Equal(got, values), "Collect(seq) = %#v, want %#v", got, values)
		})
	})
}

func TestCollectMap(t *testing.T) {
	h := check.New(t)
	h.Parallel()

	h.Run("Unit", func(h check.Harness) {
		h.Parallel()

		want := map[string]int{"a": 1, "b": 2}
		got := iterx.CollectMap(slices.Values([]pair.KeyValue[string, int]{
			pair.NewKeyValue("a", 1),
			pair.NewKeyValue("b", 2),
		}))
		h.Assertf(maps.Equal(got, want), "CollectMap(seq) = %#v, want %#v", got, want)

		wantPanic := assert.AssertionError{
			Fmt:  "precondition violation: duplicate key %v",
			Args: []any{"a"},
		}
		h.AssertPanicsWith(wantPanic, func() {
			_ = iterx.CollectMap(slices.Values([]pair.KeyValue[string, int]{
				pair.NewKeyValue("a", 1),
				pair.NewKeyValue("a", 2),
			}))
		})
	})
}

func TestLast(t *testing.T) {
	h := check.New(t)
	h.Parallel()

	h.Run("Unit", func(h check.Harness) {
		h.Parallel()

		got := iterx.Last(slices.Values([]int{1, 2, 3}))
		h.Assertf(got == 3, "Last([1 2 3]) = %d, want 3", got)

		wantPanic := assert.AssertionError{
			Fmt:  "precondition violation: %s",
			Args: []any{"iterator yielded no values"},
		}
		h.AssertPanicsWith(wantPanic, func() {
			_ = iterx.Last(iterx.Empty[int]())
		})
	})

	h.Run("Properties", func(h check.Harness) {
		h.Parallel()

		valuesGen := rapid.SliceOfN(rapid.Int(), 1, 12)
		rapid.Check(h.T(), func(t *rapid.T) {
			basic := check.NewBasic(t)
			values := valuesGen.Draw(t, "values")
			got := iterx.Last(slices.Values(values))
			want := values[len(values)-1]
			basic.Assertf(got == want, "Last(seq) = %d, want %d", got, want)
		})
	})
}

func TestFind(t *testing.T) {
	h := check.New(t)
	h.Parallel()

	h.Run("Unit", func(h check.Harness) {
		h.Parallel()

		found := iterx.Find(slices.Values([]int{1, 4, 6}), matchEven)
		h.Assertf(option.Compare(found, option.Some(4)) == 0, "Find first match = %v, want Some(4)", found)

		notFound := iterx.Find(slices.Values([]int{1, 3, 5}), matchEven)
		h.Assertf(option.Compare(notFound, option.None[int]()) == 0,
			"Find without match = %v, want None", notFound)
	})

	h.Run("Properties", func(h check.Harness) {
		h.Parallel()

		valuesGen := rapid.SliceOfN(rapid.Int(), 0, 12)
		rapid.Check(h.T(), func(t *rapid.T) {
			basic := check.NewBasic(t)
			values := valuesGen.Draw(t, "values")
			divisor := rapid.IntRange(2, 5).Draw(t, "divisor")
			match := matchDivisible(divisor)

			want := option.None[int]()
			for _, v := range values {
				mapped := match(v)
				if mapped.IsSome() {
					want = mapped
					break
				}
			}

			got := iterx.Find(slices.Values(values), match)

			basic.Assertf(option.Compare(got, want) == 0, "Find(seq) = %v, want %v", got, want)
		})
	})
}

func TestMap(t *testing.T) {
	h := check.New(t)
	h.Parallel()

	h.Run("Unit", func(h check.Harness) {
		h.Parallel()

		got := iterx.Collect(iterx.Map(slices.Values([]int{1, 2, 3}), func(v int) int {
			return v * 10
		}))
		h.Assertf(slices.Equal(got, []int{10, 20, 30}),
			"Collect(Map([1 2 3])) = %#v, want %#v", got, []int{10, 20, 30})

		calls := 0
		got = nil
		iterx.Map(slices.Values([]int{1, 2, 3}), func(v int) int {
			calls++
			return v * 10
		})(func(v int) bool {
			got = append(got, v)
			return len(got) < 2
		})
		h.Assertf(slices.Equal(got, []int{10, 20}), "short-circuit values = %#v, want %#v", got, []int{10, 20})
		h.Assertf(calls == 2, "mapping call count = %d, want 2", calls)
	})

	h.Run("Properties", func(h check.Harness) {
		h.Parallel()

		valuesGen := rapid.SliceOfN(rapid.Int(), 0, 12)
		rapid.Check(h.T(), func(t *rapid.T) {
			basic := check.NewBasic(t)
			values := valuesGen.Draw(t, "values")

			got := iterx.Collect(iterx.Map(slices.Values(values), func(v int) int {
				return v*2 + 1
			}))

			var want []int
			for _, v := range values {
				want = append(want, v*2+1)
			}

			basic.Assertf(slices.Equal(got, want), "Collect(Map(seq)) = %#v, want %#v", got, want)
		})
	})
}

func TestUnbatch(t *testing.T) {
	h := check.New(t)
	h.Parallel()

	h.Run("Unit", func(h check.Harness) {
		h.Parallel()

		stopErr := errorx.New("nostack", "stop")
		seq := iterx.Unbatch(slices.Values([]result.Result[[]int]{
			result.Success([]int{1, 2}),
			result.Failure[[]int](stopErr),
			result.Success([]int{3}),
		}))

		var values []int
		var errs []error
		for item := range seq {
			value, err := item.Get()
			if err != nil {
				errs = append(errs, err)
				continue
			}
			values = append(values, value)
		}

		h.Assertf(slices.Equal(values, []int{1, 2}), "successful values = %#v, want %#v", values, []int{1, 2})
		h.Assertf(len(errs) == 1 && errs[0] == stopErr, "errors = %#v, want [%v]", errs, stopErr)
	})

	h.Run("Properties", func(h check.Harness) {
		h.Parallel()

		batchGen := rapid.SliceOfN(rapid.Int(), 0, 4)
		batchesGen := rapid.SliceOfN(batchGen, 0, 5)
		rapid.Check(h.T(), func(t *rapid.T) {
			basic := check.NewBasic(t)
			batches := batchesGen.Draw(t, "batches")

			var batchResults []result.Result[[]int]
			var want []int
			for _, batch := range batches {
				batchResults = append(batchResults, result.Success(batch))
				want = append(want, batch...)
			}

			got := collectSuccessValues(basic, iterx.Unbatch(slices.Values(batchResults)))
			basic.Assertf(slices.Equal(got, want), "Unbatch(successes) = %#v, want %#v", got, want)
		})
	})
}

func TestFromMap(t *testing.T) {
	h := check.New(t)
	h.Parallel()

	h.Run("Unit", func(h check.Harness) {
		h.Parallel()

		empty := iterx.CollectMap(iterx.FromMap(map[string]int(nil)))
		h.Assertf(len(empty) == 0, "FromMap(nil) length = %d, want 0", len(empty))

		values := map[string]int{"a": 1, "b": 2}
		got := iterx.CollectMap(iterx.FromMap(values))
		h.Assertf(maps.Equal(got, values), "FromMap(values) = %#v, want %#v", got, values)
	})

	h.Run("Properties", func(h check.Harness) {
		h.Parallel()

		mapGen := rapid.MapOfN(rapid.Int(), rapid.Int(), 0, 8)
		rapid.Check(h.T(), func(t *rapid.T) {
			basic := check.NewBasic(t)
			values := mapGen.Draw(t, "values")
			got := iterx.CollectMap(iterx.FromMap(values))
			basic.Assertf(maps.Equal(got, values), "FromMap round-trip = %#v, want %#v", got, values)
			basic.Assertf(len(got) == len(values), "FromMap length = %d, want %d", len(got), len(values))
		})
	})
}

func TestChain(t *testing.T) {
	h := check.New(t)
	h.Parallel()

	h.Run("Unit", func(h check.Harness) {
		h.Parallel()

		empty := iterx.Collect(iterx.Chain[int]())
		h.Assertf(len(empty) == 0, "Chain() length = %d, want 0", len(empty))

		var entered []string
		seq1 := traceSeq(&entered, "seq1", 1, 2)
		seq2 := traceSeq(&entered, "seq2", 3, 4)
		seq3 := traceSeq(&entered, "seq3", 5)

		var got []int
		iterx.Chain(seq1, seq2, seq3)(func(v int) bool {
			got = append(got, v)
			return len(got) < 3
		})

		h.Assertf(slices.Equal(got, []int{1, 2, 3}), "short-circuit values = %#v, want %#v", got, []int{1, 2, 3})
		h.Assertf(slices.Equal(entered, []string{"seq1", "seq2"}),
			"entered sequences = %#v, want %#v", entered, []string{"seq1", "seq2"})
	})

	h.Run("Properties", func(h check.Harness) {
		h.Parallel()

		valuesGen := rapid.SliceOfN(rapid.Int(), 0, 8)
		rapid.Check(h.T(), func(t *rapid.T) {
			basic := check.NewBasic(t)
			first := valuesGen.Draw(t, "first")
			second := valuesGen.Draw(t, "second")
			third := valuesGen.Draw(t, "third")

			got := iterx.Collect(iterx.Chain(slices.Values(first), slices.Values(second), slices.Values(third)))

			var want []int
			want = append(want, first...)
			want = append(want, second...)
			want = append(want, third...)

			basic.Assertf(slices.Equal(got, want), "Chain(seq...) = %#v, want %#v", got, want)
		})
	})
}

func matchEven(v int) option.Option[int] {
	if v%2 == 0 {
		return option.Some(v)
	}
	return option.None[int]()
}

func matchDivisible(divisor int) func(int) option.Option[int] {
	return func(v int) option.Option[int] {
		if v%divisor == 0 {
			return option.Some(v*3 + 1)
		}
		return option.None[int]()
	}
}

func traceSeq(entered *[]string, name string, values ...int) iter.Seq[int] {
	return func(yield func(int) bool) {
		*entered = append(*entered, name)
		for _, v := range values {
			if !yield(v) {
				return
			}
		}
	}
}

func collectSuccessValues[T any](h check.BasicHarness, seq iter.Seq[result.Result[T]]) []T {
	var values []T
	for item := range seq {
		value, err := item.Get()
		h.NoErrorf(err, "expected success result")
		values = append(values, value)
	}
	return values
}
