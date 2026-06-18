package util

import (
	"testing"
)

func TestDefaultMap_Get_CreatesOnMiss(t *testing.T) {
	factoryCalls := 0
	dm := NewDefaultMap[string](func() int {
		factoryCalls++
		return 42
	})

	result := dm.Get("missing")

	if result != 42 {
		t.Errorf("expected 42, got %d", result)
	}
	if factoryCalls != 1 {
		t.Errorf("expected factory to be called once, was called %d times", factoryCalls)
	}

	// Verify it was stored
	if !dm.Contains("missing") {
		t.Error("expected key to be stored after Get")
	}
}

func TestDefaultMap_Get_ReturnsExisting(t *testing.T) {
	factoryCalls := 0
	dm := NewDefaultMap[string](func() int {
		factoryCalls++
		return 42
	})

	dm.Set("existing", 100)
	result := dm.Get("existing")

	if result != 100 {
		t.Errorf("expected 100, got %d", result)
	}
	if factoryCalls != 0 {
		t.Errorf("expected factory not to be called, was called %d times", factoryCalls)
	}
}

func TestDefaultMap_Peek_ReturnsFalseOnMiss(t *testing.T) {
	dm := NewDefaultMap[string](func() int { return 42 })

	val, ok := dm.Peek("missing")

	if ok {
		t.Error("expected ok to be false for missing key")
	}
	if val != 0 {
		t.Errorf("expected zero value, got %d", val)
	}
}

func TestDefaultMap_Peek_ReturnsTrueOnHit(t *testing.T) {
	dm := NewDefaultMap[string](func() int { return 42 })
	dm.Set("existing", 100)

	val, ok := dm.Peek("existing")

	if !ok {
		t.Error("expected ok to be true for existing key")
	}
	if val != 100 {
		t.Errorf("expected 100, got %d", val)
	}
}

func TestDefaultMap_Set_OverwritesExisting(t *testing.T) {
	dm := NewDefaultMap[string](func() int { return 42 })
	dm.Set("key", 100)
	dm.Set("key", 200)

	val, ok := dm.Peek("key")

	if !ok {
		t.Error("expected key to exist")
	}
	if val != 200 {
		t.Errorf("expected 200, got %d", val)
	}
}

func TestDefaultMap_Delete_RemovesKey(t *testing.T) {
	dm := NewDefaultMap[string](func() int { return 42 })
	dm.Set("key", 100)

	dm.Delete("key")

	if dm.Contains("key") {
		t.Error("expected key to be deleted")
	}
}

func TestDefaultMap_Delete_NoOpForMissingKey(t *testing.T) {
	dm := NewDefaultMap[string](func() int { return 42 })

	// Should not panic
	dm.Delete("nonexistent")
}

func TestDefaultMap_Contains_TrueForExisting(t *testing.T) {
	dm := NewDefaultMap[string](func() int { return 42 })
	dm.Set("key", 100)

	if !dm.Contains("key") {
		t.Error("expected Contains to return true for existing key")
	}
}

func TestDefaultMap_Contains_FalseForMissing(t *testing.T) {
	dm := NewDefaultMap[string](func() int { return 42 })

	if dm.Contains("missing") {
		t.Error("expected Contains to return false for missing key")
	}
}

func TestDefaultMap_Keys_ReturnsAllKeys(t *testing.T) {
	dm := NewDefaultMap[string](func() int { return 0 })
	dm.Set("a", 1)
	dm.Set("b", 2)
	dm.Set("c", 3)

	keys := dm.Keys()

	if len(keys) != 3 {
		t.Errorf("expected 3 keys, got %d", len(keys))
	}

	keySet := make(map[string]bool)
	for _, k := range keys {
		keySet[k] = true
	}

	for _, expected := range []string{"a", "b", "c"} {
		if !keySet[expected] {
			t.Errorf("expected key %q to be present", expected)
		}
	}
}

func TestDefaultMap_Keys_EmptyMap(t *testing.T) {
	dm := NewDefaultMap[string](func() int { return 0 })

	keys := dm.Keys()

	if len(keys) != 0 {
		t.Errorf("expected 0 keys, got %d", len(keys))
	}
}

func TestDefaultMap_Values_ReturnsAllValues(t *testing.T) {
	dm := NewDefaultMap[string](func() int { return 0 })
	dm.Set("a", 1)
	dm.Set("b", 2)
	dm.Set("c", 3)

	values := dm.Values()

	if len(values) != 3 {
		t.Errorf("expected 3 values, got %d", len(values))
	}

	sum := 0
	for _, v := range values {
		sum += v
	}
	if sum != 6 {
		t.Errorf("expected sum of values to be 6, got %d", sum)
	}
}

func TestDefaultMap_Values_EmptyMap(t *testing.T) {
	dm := NewDefaultMap[string](func() int { return 0 })

	values := dm.Values()

	if len(values) != 0 {
		t.Errorf("expected 0 values, got %d", len(values))
	}
}

func TestDefaultMap_WithPointerValues(t *testing.T) {
	type data struct {
		value int
	}

	dm := NewDefaultMap[string](func() *data {
		return &data{value: 0}
	})

	// Get creates a new pointer
	d1 := dm.Get("key")
	d1.value = 42

	// Get same key returns same pointer
	d2 := dm.Get("key")
	if d2.value != 42 {
		t.Errorf("expected 42, got %d", d2.value)
	}
	if d1 != d2 {
		t.Error("expected same pointer to be returned")
	}
}

func TestDefaultMap_WithNestedMaps(t *testing.T) {
	dm := NewDefaultMap[string](func() *DefaultMap[string, int] {
		return NewDefaultMap[string](func() int { return 0 })
	})

	dm.Get("outer").Set("inner", 42)

	result := dm.Get("outer").Get("inner")
	if result != 42 {
		t.Errorf("expected 42, got %d", result)
	}
}
