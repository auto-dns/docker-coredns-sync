package util

// Map applies the given function to each element in the slice and returns a new slice with the results
func Map[T any, R any](slice []T, f func(T) R) []R {
	result := make([]R, len(slice))
	for i, v := range slice {
		result[i] = f(v)
	}
	return result
}

func Filter[T any](slice []T, f func(T) bool) []T {
	var result []T
	for _, v := range slice {
		if keep := f(v); keep {
			result = append(result, v)
		}
	}
	return result
}

func Reverse[T any](slice []T) []T {
	reversed := make([]T, len(slice))
	length := len(slice)
	for i, v := range slice {
		reversed[length-1-i] = v
	}
	return reversed
}
