package iterx

import (
	"iter"

	"github.com/typesanitizer/happygo/common/assert"
	"github.com/typesanitizer/happygo/common/core/option"
	"github.com/typesanitizer/happygo/common/core/pair"
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

// Last returns the last yielded value from seq.
//
// Pre-condition: seq yields at least once.
func Last[T any](seq iter.Seq[T]) T {
	var last T
	sawValue := false
	for v := range seq {
		last = v
		sawValue = true
	}
	assert.Precondition(sawValue, "iterator yielded no values")
	return last
}

// Find returns the first Some value produced by f over seq.
func Find[A any, B any](seq iter.Seq[A], f func(A) option.Option[B]) option.Option[B] {
	for v := range seq {
		result := f(v)
		if result.IsSome() {
			return result
		}
	}
	return option.None[B]()
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

func FromMap[K comparable, V any](kvs map[K]V) iter.Seq[pair.KeyValue[K, V]] {
	return func(yield func(pair.KeyValue[K, V]) bool) {
		for k, v := range kvs {
			if !yield(pair.NewKeyValue(k, v)) {
				return
			}
		}
	}
}
