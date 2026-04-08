package iterx

import (
	"iter"

	"github.com/typesanitizer/happygo/common/core/result"
)

// Collect accumulates all values from an iterator into a slice.
func Collect[T any](seq iter.Seq[T]) []T {
	var result []T
	for v := range seq {
		result = append(result, v)
	}
	return result
}

// Map transforms each value in seq with f.
func Map[T any, U any](seq iter.Seq[T], f func(T) U) iter.Seq[U] {
	return func(yield func(U) bool) {
		for v := range seq {
			if !yield(f(v)) {
				return
			}
		}
	}
}

// Unbatch flattens an iterator of batched results into an iterator of element
// results, preserving failures.
func Unbatch[T any](seq iter.Seq[result.Result[[]T]]) iter.Seq[result.Result[T]] {
	return func(yield func(result.Result[T]) bool) {
		for batchRes := range seq {
			batch, err := batchRes.Get()
			if err != nil {
				yield(result.Failure[T](err))
				return
			}
			for _, item := range batch {
				if !yield(result.Success(item)) {
					return
				}
			}
		}
	}
}
