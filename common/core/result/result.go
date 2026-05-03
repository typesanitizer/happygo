// Copyright 2026 Varun Gandhi
//
// SPDX-License-Identifier: PolyForm-Noncommercial-1.0.0

// Package result provides a generic Result[T] type holding either a value or
// an error.
package result

import "github.com/typesanitizer/happygo/common/assert"

// Result holds either a value of type T or an error.
//
// Use [Success] or [Failure] to construct one. A Failure always carries a
// non-nil error; a Success has a nil error.
type Result[T any] struct {
	value T
	err   error
}

// Success returns a Result containing value.
func Success[T any](value T) Result[T] {
	return Result[T]{value: value, err: nil}
}

// Failure returns a Result containing err.
//
// Pre-condition: err is non-nil.
func Failure[T any](err error) Result[T] {
	assert.Preconditionf(err != nil, "Failure called with nil error")
	var zero T
	return Result[T]{value: zero, err: err}
}

// Get returns the contained value and error.
// If r is a Success, err is nil.
// If r is a Failure, err is non-nil and the value is the zero value of T.
func (r Result[T]) Get() (T, error) {
	return r.value, r.err
}

// ErrOrNil returns the contained error, or nil if r is a Success.
func (r Result[T]) ErrOrNil() error {
	return r.err
}
