package util

type DefaultMap[K comparable, V any] struct {
	internal map[K]V
	factory  func() V
}

func NewDefaultMap[K comparable, V any](factory func() V) *DefaultMap[K, V] {
	return &DefaultMap[K, V]{
		internal: make(map[K]V),
		factory:  factory,
	}
}

// Modifies the map
func (d *DefaultMap[K, V]) Get(key K) V {
	if val, ok := d.internal[key]; ok {
		return val
	}
	val := d.factory()
	d.internal[key] = val
	return val
}

func (d *DefaultMap[K, V]) Peek(key K) (V, bool) {
	val, ok := d.internal[key]
	return val, ok
}

func (d *DefaultMap[K, V]) Contains(key K) bool {
	_, ok := d.internal[key]
	return ok
}

func (d *DefaultMap[K, V]) Set(key K, value V) {
	d.internal[key] = value
}

func (d *DefaultMap[K, V]) Delete(key K) {
	delete(d.internal, key)
}

func (d *DefaultMap[K, V]) Keys() []K {
	keys := make([]K, 0, len(d.internal))
	for k := range d.internal {
		keys = append(keys, k)
	}
	return keys
}

func (d *DefaultMap[K, V]) Values() []V {
	values := make([]V, 0, len(d.internal))
	for _, v := range d.internal {
		values = append(values, v)
	}
	return values
}
