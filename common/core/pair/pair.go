// Copyright 2026 Varun Gandhi
//
// SPDX-License-Identifier: PolyForm-Noncommercial-1.0.0

package pair

type KeyValue[K, V any] struct {
	Key   K
	Value V
}

func NewKeyValue[K, V any](k K, v V) KeyValue[K, V] {
	return KeyValue[K, V]{k, v}
}
