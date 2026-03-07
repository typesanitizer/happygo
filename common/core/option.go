// Package core provides foundational generic types.
package core

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
func (o *Option[T]) Get() (T, bool) {
	return o.value, o.valid
}

// IsSome reports whether the Option contains a value.
func (o *Option[T]) IsSome() bool {
	return o.valid
}

// ValueOr returns the contained value if present, otherwise fallback.
func (o *Option[T]) ValueOr(fallback T) T {
	if o.valid {
		return o.value
	}
	return fallback
}
