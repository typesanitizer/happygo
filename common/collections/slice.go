// Copyright 2026 Varun Gandhi
//
// SPDX-License-Identifier: PolyForm-Noncommercial-1.0.0

package collections

// FilterSlice returns a new slice containing only elements for which pred returns true.
func FilterSlice[T any](s []T, pred func(T) bool) []T {
	var result []T
	for _, v := range s {
		if pred(v) {
			result = append(result, v)
		}
	}
	return result
}
