package cmpx

// CompareBool implements false < true
func CompareBool(b1 bool, b2 bool) int {
	if b1 {
		if b2 {
			return 0
		}
		return 1
	}
	if b2 {
		return -1
	}
	return 0
}
