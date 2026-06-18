package util

import (
	"testing"
)

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
