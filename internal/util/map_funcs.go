package util

func FirstValue[K comparable, V any](m map[K]V) (V, bool) {
	for _, v := range m {
		return v, true
	}
	var zero V
	return zero, false
}
