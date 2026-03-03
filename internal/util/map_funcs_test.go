package util

import (
	"testing"
)

func TestFirstValue_ReturnsAValue(t *testing.T) {
	m := map[string]int{
		"a": 1,
		"b": 2,
		"c": 3,
	}

	val, ok := FirstValue(m)

	if !ok {
		t.Error("expected ok to be true for non-empty map")
	}

	// Since map iteration order is not guaranteed, we just check that
	// the returned value is one of the values in the map
	found := false
	for _, v := range m {
		if v == val {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("returned value %d is not in the map", val)
	}
}

func TestFirstValue_EmptyMap(t *testing.T) {
	m := map[string]int{}

	val, ok := FirstValue(m)

	if ok {
		t.Error("expected ok to be false for empty map")
	}
	if val != 0 {
		t.Errorf("expected zero value for int, got %d", val)
	}
}

func TestFirstValue_SingleElement(t *testing.T) {
	m := map[string]int{
		"only": 42,
	}

	val, ok := FirstValue(m)

	if !ok {
		t.Error("expected ok to be true")
	}
	if val != 42 {
		t.Errorf("expected 42, got %d", val)
	}
}

func TestFirstValue_WithStringValues(t *testing.T) {
	m := map[int]string{
		1: "one",
		2: "two",
	}

	val, ok := FirstValue(m)

	if !ok {
		t.Error("expected ok to be true")
	}
	if val != "one" && val != "two" {
		t.Errorf("expected 'one' or 'two', got %q", val)
	}
}

func TestFirstValue_WithPointerValues(t *testing.T) {
	type data struct{ value int }
	d1 := &data{value: 1}
	d2 := &data{value: 2}

	m := map[string]*data{
		"a": d1,
		"b": d2,
	}

	val, ok := FirstValue(m)

	if !ok {
		t.Error("expected ok to be true")
	}
	if val != d1 && val != d2 {
		t.Error("expected one of the stored pointers")
	}
}

func TestFirstValue_NilMap(t *testing.T) {
	var m map[string]int

	val, ok := FirstValue(m)

	if ok {
		t.Error("expected ok to be false for nil map")
	}
	if val != 0 {
		t.Errorf("expected zero value, got %d", val)
	}
}
