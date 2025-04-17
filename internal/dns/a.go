package dns

import (
	"fmt"
	"net"
)

type ARecord struct {
	DnsRecord
}

func NewARecord(name, value string) (*ARecord, error) {
	if !isValidHostname(name) {
		return nil, fmt.Errorf("invalid hostname for A record: %s", name)
	}
	if net.ParseIP(value) == nil {
		return nil, fmt.Errorf("invalid IP address: %s", value)
	}
	return &ARecord{
		DnsRecord: DnsRecord{
			Name:  name,
			Type:  "A",
			Value: value,
		},
	}, nil
}

func (a *ARecord) Equal(other Record) bool {
	o, ok := other.(*ARecord)
	if !ok {
		return false
	}
	return a.DnsRecord.Equal(o.DnsRecord)
}
