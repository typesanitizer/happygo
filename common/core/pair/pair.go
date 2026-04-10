package pair

type KeyValue[K, V any] struct {
	Key   K
	Value V
}

func NewKeyValue[K, V any](k K, v V) KeyValue[K, V] {
	return KeyValue[K, V]{k, v}
}
