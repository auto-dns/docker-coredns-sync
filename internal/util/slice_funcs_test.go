package util

import (
	"strconv"
	"testing"
)

func TestMap_TransformsElements(t *testing.T) {
	input := []int{1, 2, 3, 4, 5}
	square := func(n int) int { return n * n }

	result := Map(input, square)

	expected := []int{1, 4, 9, 16, 25}
	if len(result) != len(expected) {
		t.Fatalf("expected length %d, got %d", len(expected), len(result))
	}
	for i, v := range result {
		if v != expected[i] {
			t.Errorf("at index %d: expected %d, got %d", i, expected[i], v)
		}
	}
}

func TestMap_EmptySlice(t *testing.T) {
	input := []int{}
	double := func(n int) int { return n * 2 }

	result := Map(input, double)

	if len(result) != 0 {
		t.Errorf("expected empty slice, got length %d", len(result))
	}
}

func TestMap_TypeConversion(t *testing.T) {
	input := []int{1, 2, 3}
	toString := func(n int) string { return strconv.Itoa(n) }

	result := Map(input, toString)

	expected := []string{"1", "2", "3"}
	if len(result) != len(expected) {
		t.Fatalf("expected length %d, got %d", len(expected), len(result))
	}
	for i, v := range result {
		if v != expected[i] {
			t.Errorf("at index %d: expected %q, got %q", i, expected[i], v)
		}
	}
}

func TestMap_PreservesOrder(t *testing.T) {
	input := []string{"a", "b", "c", "d"}
	identity := func(s string) string { return s }

	result := Map(input, identity)

	for i, v := range result {
		if v != input[i] {
			t.Errorf("at index %d: expected %q, got %q", i, input[i], v)
		}
	}
}

func TestFilter_KeepsMatching(t *testing.T) {
	input := []int{1, 2, 3, 4, 5, 6}
	isEven := func(n int) bool { return n%2 == 0 }

	result := Filter(input, isEven)

	expected := []int{2, 4, 6}
	if len(result) != len(expected) {
		t.Fatalf("expected length %d, got %d", len(expected), len(result))
	}
	for i, v := range result {
		if v != expected[i] {
			t.Errorf("at index %d: expected %d, got %d", i, expected[i], v)
		}
	}
}

func TestFilter_EmptyResult(t *testing.T) {
	input := []int{1, 3, 5, 7}
	isEven := func(n int) bool { return n%2 == 0 }

	result := Filter(input, isEven)

	if len(result) != 0 {
		t.Errorf("expected empty slice, got length %d", len(result))
	}
}

func TestFilter_AllMatch(t *testing.T) {
	input := []int{2, 4, 6, 8}
	isEven := func(n int) bool { return n%2 == 0 }

	result := Filter(input, isEven)

	if len(result) != len(input) {
		t.Fatalf("expected length %d, got %d", len(input), len(result))
	}
	for i, v := range result {
		if v != input[i] {
			t.Errorf("at index %d: expected %d, got %d", i, input[i], v)
		}
	}
}

func TestFilter_EmptyInput(t *testing.T) {
	input := []int{}
	always := func(n int) bool { return true }

	result := Filter(input, always)

	if len(result) != 0 {
		t.Errorf("expected empty slice, got length %d", len(result))
	}
}

func TestFilter_PreservesOrder(t *testing.T) {
	input := []string{"apple", "banana", "apricot", "blueberry", "avocado"}
	startsWithA := func(s string) bool { return len(s) > 0 && s[0] == 'a' }

	result := Filter(input, startsWithA)

	expected := []string{"apple", "apricot", "avocado"}
	if len(result) != len(expected) {
		t.Fatalf("expected length %d, got %d", len(expected), len(result))
	}
	for i, v := range result {
		if v != expected[i] {
			t.Errorf("at index %d: expected %q, got %q", i, expected[i], v)
		}
	}
}

func TestReverse_ReversesOrder(t *testing.T) {
	input := []int{1, 2, 3, 4, 5}

	result := Reverse(input)

	expected := []int{5, 4, 3, 2, 1}
	if len(result) != len(expected) {
		t.Fatalf("expected length %d, got %d", len(expected), len(result))
	}
	for i, v := range result {
		if v != expected[i] {
			t.Errorf("at index %d: expected %d, got %d", i, expected[i], v)
		}
	}
}

func TestReverse_SingleElement(t *testing.T) {
	input := []int{42}

	result := Reverse(input)

	if len(result) != 1 {
		t.Fatalf("expected length 1, got %d", len(result))
	}
	if result[0] != 42 {
		t.Errorf("expected 42, got %d", result[0])
	}
}

func TestReverse_EmptySlice(t *testing.T) {
	input := []int{}

	result := Reverse(input)

	if len(result) != 0 {
		t.Errorf("expected empty slice, got length %d", len(result))
	}
}

func TestReverse_TwoElements(t *testing.T) {
	input := []string{"first", "second"}

	result := Reverse(input)

	if len(result) != 2 {
		t.Fatalf("expected length 2, got %d", len(result))
	}
	if result[0] != "second" || result[1] != "first" {
		t.Errorf("expected [second, first], got %v", result)
	}
}

func TestReverse_DoesNotModifyOriginal(t *testing.T) {
	input := []int{1, 2, 3}
	originalCopy := make([]int, len(input))
	copy(originalCopy, input)

	_ = Reverse(input)

	for i, v := range input {
		if v != originalCopy[i] {
			t.Errorf("original slice was modified at index %d: expected %d, got %d", i, originalCopy[i], v)
		}
	}
}

func TestReverse_Strings(t *testing.T) {
	input := []string{"a", "b", "c"}

	result := Reverse(input)

	expected := []string{"c", "b", "a"}
	for i, v := range result {
		if v != expected[i] {
			t.Errorf("at index %d: expected %q, got %q", i, expected[i], v)
		}
	}
}
