package core

import "github.com/typesanitizer/happygo/common/core/result"

// Result holds either a value of type T or an error.
//
// Use [Success] or [Failure] to construct one. A Failure always carries a
// non-nil error; a Success has a nil error.
type Result[T any] = result.Result[T]

// Success returns a Result containing value.
func Success[T any](value T) Result[T] {
	return result.Success(value)
}

// Failure returns a Result containing err.
//
// Pre-condition: err is non-nil.
func Failure[T any](err error) Result[T] {
	return result.Failure[T](err)
}
