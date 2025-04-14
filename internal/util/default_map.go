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

func (d *DefaultMap[K, V]) Get(key K) V {
	if val, ok := d.internal[key]; ok {
		return val
	}
	val := d.factory()
	d.internal[key] = val
	return val
}

func (d *DefaultMap[K, V]) Set(key K, value V) {
	d.internal[key] = value
}

func (d *DefaultMap[K, V]) Items() map[K]V {
	return d.internal
}
