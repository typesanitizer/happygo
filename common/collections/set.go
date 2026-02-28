package collections

import "iter"

// Set is a basic mutable set.
type Set[T comparable] struct {
	values map[T]struct{}
}

// NewSet returns an empty set.
func NewSet[T comparable]() Set[T] {
	return Set[T]{values: make(map[T]struct{})}
}

// Insert adds value to the set.
// It returns true if a value was inserted.
func (s *Set[T]) Insert(value T) bool {
	before := len(s.values)
	s.values[value] = struct{}{}
	return len(s.values) != before
}

// Contains reports whether value is in the set.
func (s *Set[T]) Contains(value T) bool {
	_, ok := s.values[value]
	return ok
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
