// Copyright 2026 Varun Gandhi
//
// SPDX-License-Identifier: PolyForm-Noncommercial-1.0.0

package collections

import (
	"iter"

	"github.com/typesanitizer/happygo/common/core/op"
	"github.com/typesanitizer/happygo/common/core/option"
)

// MonotoneMap is a mutable map that preserves insertion order of keys,
// allowing for deterministic iteration order.
//
// It does not support in-place deletion. To drop keys, build a filtered copy
// with CloneWithout.
type MonotoneMap[K comparable, V any] struct {
	keys   []K
	values map[K]V
}

// NewMonotoneMap returns an empty monotone map.
func NewMonotoneMap[K comparable, V any]() MonotoneMap[K, V] {
	return MonotoneMap[K, V]{
		keys:   nil,
		values: map[K]V{},
	}
}

// CloneWithout returns a shallow clone of m with keys in omit removed, if
// present.
//
// Keys in omit that are not present in m are ignored.
//
// Keys and values are copied using assignment, so this is a shallow clone.
//
// Time: Θ(|m|). Additional space: Θ(|m|) in the worst case.
func (m MonotoneMap[K, V]) CloneWithout(omit Set[K]) MonotoneMap[K, V] {
	keys := make([]K, 0, m.Len())
	values := make(map[K]V, len(m.values))
	for _, key := range m.keys {
		if omit.Contains(key) {
			continue
		}
		value := m.values[key]
		keys = append(keys, key)
		values[key] = value
	}
	return MonotoneMap[K, V]{keys: keys, values: values}
}

// Lookup returns the value for key, if present.
//
// Expected time: Θ(1).
func (m MonotoneMap[K, V]) Lookup(key K) option.Option[V] {
	value, ok := m.values[key]
	return option.NewOption(value, ok)
}

// Len returns the number of entries.
func (m MonotoneMap[K, V]) Len() int {
	return len(m.values)
}

// Keys returns the keys in insertion order.
//
// Creating the iterator is Θ(1). Exhausting it is Θ(|m|).
func (m MonotoneMap[K, V]) Keys() iter.Seq[K] {
	return func(yield func(K) bool) {
		for _, key := range m.keys {
			if !yield(key) {
				return
			}
		}
	}
}

// InsertOrKeep inserts the value if the key is absent.
//
// Expected time: Θ(1).
func (m *MonotoneMap[K, V]) InsertOrKeep(key K, value V) op.InsertResult {
	if _, ok := m.values[key]; ok {
		return op.KeptOld
	}
	m.keys = append(m.keys, key)
	m.values[key] = value
	return op.InsertedNew
}

// InsertOrReplace inserts or replaces the value, returning the old value if
// one existed.
//
// Expected time: Θ(1).
func (m *MonotoneMap[K, V]) InsertOrReplace(key K, value V) option.Option[V] {
	old, ok := m.values[key]
	if !ok {
		m.keys = append(m.keys, key)
	}
	m.values[key] = value
	return option.NewOption(old, ok)
}
