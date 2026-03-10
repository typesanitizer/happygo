package iterx

import "iter"

// Collect accumulates all values from an iterator into a slice.
func Collect[T any](seq iter.Seq[T]) []T {
	var result []T
	for v := range seq {
		result = append(result, v)
	}
	return result
}
