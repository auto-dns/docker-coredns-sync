package dns

import (
	"fmt"
)

type DnsRecord struct {
	Name       string
	RecordType string
	Value      string
}

func (dr DnsRecord) GetName() string {
	return dr.Name
}

func (dr DnsRecord) GetRecordType() string {
	return dr.RecordType
}

func (dr DnsRecord) GetValue() string {
	return dr.Value
}

func (dr DnsRecord) Render() string {
	if dr.Value == "" {
		return fmt.Sprintf("%s -> <no value>", dr.Name)
	}
	return fmt.Sprintf("%s -> %s", dr.Name, dr.Value)
}

func (dr DnsRecord) Equal(other DnsRecord) bool {
	return dr.Name == other.Name && dr.RecordType == other.RecordType && dr.Value == other.Value
}
