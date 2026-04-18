package collections

import (
	"cmp"
	"slices"
	"sort"
)

// SortedMapKeys returns the keys of a map in sorted order.
func SortedMapKeys[K cmp.Ordered, V any](m map[K]V) []K {
	keys := make([]K, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	return keys
}

func SortedMapKeysFunc[K comparable, V any](m map[K]V, cmp func(k1, k2 K) int) []K {
	keys := make([]K, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.SortFunc(keys, cmp)
	return keys
}

// SortedMapValues returns the values of a map in sorted order.
func SortedMapValues[K comparable, V cmp.Ordered](m map[K]V) []V {
	values := make([]V, 0, len(m))
	for _, v := range m {
		values = append(values, v)
	}
	sort.Slice(values, func(i, j int) bool { return values[i] < values[j] })
	return values
}
