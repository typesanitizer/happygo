// Copyright 2026 Varun Gandhi
//
// SPDX-License-Identifier: PolyForm-Noncommercial-1.0.0

// Package core provides foundational generic types.
package core

import "github.com/typesanitizer/happygo/common/core/option"

// Option represents a value that may or may not be present.
type Option[T any] = option.Option[T]

// NewOption returns Some(v) if ok is true, otherwise None.
func NewOption[T any](v T, ok bool) Option[T] {
	return option.NewOption(v, ok)
}

// Some returns an Option containing v.
func Some[T any](v T) Option[T] {
	return option.Some(v)
}

// None returns an empty Option.
func None[T any]() Option[T] {
	return option.None[T]()
}
