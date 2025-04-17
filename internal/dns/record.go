package dns

type Record interface {
	GetName() string
	GetType() string
	GetValue() string
	Render() string
	Key() string
	Equal(other Record) bool
}
