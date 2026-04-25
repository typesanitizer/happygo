package collections

import "github.com/typesanitizer/happygo/common/assert"

// Stack is a simple LIFO stack.
type Stack[T any] struct {
	values []T
}

func NewStack[T any]() Stack[T] {
	return Stack[T]{values: nil}
}

func (s *Stack[T]) Len() int {
	return len(s.values)
}

// Push adds value to the top of the stack.
func (s *Stack[T]) Push(value T) {
	s.values = append(s.values, value)
}

// Pop removes and returns the top element.
//
// Pre-condition: the stack is non-empty.
func (s *Stack[T]) Pop() T {
	l := s.Len()
	assert.Preconditionf(l > 0, "Pop on empty stack")
	n := l - 1
	value := s.values[n]
	s.values = s.values[:n]
	return value
}

// IsEmpty reports whether the stack has no elements.
func (s *Stack[T]) IsEmpty() bool {
	return s.Len() == 0
}
