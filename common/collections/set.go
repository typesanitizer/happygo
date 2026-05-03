// Copyright 2026 Varun Gandhi
//
// SPDX-License-Identifier: PolyForm-Noncommercial-1.0.0

package collections

import (
	"cmp"
	"iter"
	"sort"

	"github.com/typesanitizer/happygo/common/assert"
	"github.com/typesanitizer/happygo/common/core/op"
)

// Set is a basic mutable set.
type Set[T comparable] struct {
	values map[T]struct{}
}

// NewSet returns an empty set.
func NewSet[T comparable]() Set[T] {
	return Set[T]{values: make(map[T]struct{})}
}

// Insert adds value to the set.
func (s *Set[T]) Insert(value T) op.InsertResult {
	if _, ok := s.values[value]; ok {
		return op.KeptOld
	}
	s.values[value] = struct{}{}
	return op.InsertedNew
}

// InsertNew adds value to the set.
//
// Pre-condition: value is not already present.
func (s *Set[T]) InsertNew(value T) {
	_, ok := s.values[value]
	assert.Preconditionf(!ok, "set already contains value %v", value)
	s.values[value] = struct{}{}
}

// Contains reports whether value is in the set.
func (s *Set[T]) Contains(value T) bool {
	_, ok := s.values[value]
	return ok
}

// Len returns the number of elements in the set.
func (s *Set[T]) Len() int {
	return len(s.values)
}

// IsSubsetOf reports whether every element of s is also in bigger.
func (s *Set[T]) IsSubsetOf(bigger *Set[T]) bool {
	if s.Len() > bigger.Len() {
		return false
	}
	for value := range s.values {
		if !bigger.Contains(value) {
			return false
		}
	}
	return true
}

// SortedValues returns the elements of a set with an ordered element type in sorted order.
func SortedValues[T cmp.Ordered](s Set[T]) []T {
	values := make([]T, 0, s.Len())
	for v := range s.values {
		values = append(values, v)
	}
	sort.Slice(values, func(i, j int) bool { return values[i] < values[j] })
	return values
}

// ValuesNonDet returns all set values in unspecified iteration order.
func (s *Set[T]) ValuesNonDet() iter.Seq[T] {
	return func(yield func(T) bool) {
		for value := range s.values {
			if !yield(value) {
				return
			}
		}
	}
}
