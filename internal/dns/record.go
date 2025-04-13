package dns

type Record interface {
	GetName() string
	GetRecordType() string
	GetValue() string
	Render() string
	Equal(other Record) bool
}
