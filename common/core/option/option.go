// Copyright 2026 Varun Gandhi
//
// SPDX-License-Identifier: PolyForm-Noncommercial-1.0.0

// Package option provides a generic optional value type.
package option

import (
	"cmp"

	"github.com/typesanitizer/happygo/common/assert"
	"github.com/typesanitizer/happygo/common/cmpx"
)

// Option represents a value that may or may not be present.
type Option[T any] struct {
	value T
	valid bool
}

// NewOption returns Some(v) if ok is true, otherwise None.
func NewOption[T any](v T, ok bool) Option[T] {
	if ok {
		return Option[T]{value: v, valid: true}
	}
	return None[T]()
}

// Some returns an Option containing v.
func Some[T any](v T) Option[T] {
	return Option[T]{value: v, valid: true}
}

// None returns an empty Option.
func None[T any]() Option[T] {
	var zero T
	return Option[T]{value: zero, valid: false}
}

// Get returns the value and whether it is present.
func (o Option[T]) Get() (T, bool) {
	return o.value, o.valid
}

// Unwrap returns the contained value.
//
// Pre-condition: the Option is Some.
func (o Option[T]) Unwrap() T {
	assert.Preconditionf(o.valid, "called Unwrap on None")
	return o.value
}

// IsSome reports whether the Option contains a value.
func (o Option[T]) IsSome() bool {
	return o.valid
}

func (o Option[T]) IsNone() bool {
	return !o.valid
}

// ValueOr returns the contained value if present, otherwise fallback.
func (o Option[T]) ValueOr(fallback T) T {
	if o.valid {
		return o.value
	}
	return fallback
}

// Compare o1 o2 ensures the following ordering:
// 1. None == None
// 2. None < Some(v)
// 3. Some(v1) < Some(v2) iff v1 < v2
func Compare[T cmp.Ordered](o1 Option[T], o2 Option[T]) int {
	v1, ok1 := o1.Get()
	v2, ok2 := o2.Get()
	if ok1 && ok2 {
		return cmp.Compare(v1, v2)
	}
	return cmpx.CompareBool(ok1, ok2)
}
