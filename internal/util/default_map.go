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

// Does not modify the map
func (d *DefaultMap[K, V]) GetOrDefault(key K, defaultVal V) V {
	if val, ok := d.internal[key]; ok {
		return val
	}
	return defaultVal
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

func (d *DefaultMap[K, V]) Items() map[K]V {
	return d.internal
}

func (d *DefaultMap[K, V]) Values() []V {
	values := make([]V, 0, len(d.internal))
	for _, v := range d.internal {
		values = append(values, v)
	}
	return values
}
