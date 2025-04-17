package dns

import (
	"fmt"
)

type DnsRecord struct {
	Name  string
	Type  string
	Value string
}

func (dr DnsRecord) GetName() string {
	return dr.Name
}

func (dr DnsRecord) GetType() string {
	return dr.Type
}

func (dr DnsRecord) GetValue() string {
	return dr.Value
}

func (dr DnsRecord) Render() string {
	if dr.Value == "" {
		return fmt.Sprintf("[%s] %s -> <no value>", dr.Type, dr.Name)
	}
	return fmt.Sprintf("[%s] %s -> %s", dr.Type, dr.Name, dr.Value)
}

func (dr DnsRecord) Equal(other DnsRecord) bool {
	return dr.Name == other.Name && dr.Type == other.Type && dr.Value == other.Value
}

func (dr DnsRecord) Key() string {
	return fmt.Sprintf("%s|%s|%s", dr.Name, dr.Type, dr.Value)
}
