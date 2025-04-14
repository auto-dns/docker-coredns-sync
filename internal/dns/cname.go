package dns

import (
	"fmt"
	"regexp"
)

type CNAMERecord struct {
	DnsRecord
}

var hostnameRegexp = regexp.MustCompile(`^[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(?:\.[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*$`)

func NewCNAMERecord(name, value string) (*CNAMERecord, error) {
	if !isValidHostname(name) {
		return nil, fmt.Errorf("invalid hostname for CNAME record (name): %s", name)
	}
	if !isValidHostname(value) {
		return nil, fmt.Errorf("invalid hostname for CNAME record (value): %s", value)
	}
	return &CNAMERecord{
		DnsRecord: DnsRecord{
			Name:       name,
			RecordType: "CNAME",
			Value:      value,
		},
	}, nil
}

func (c *CNAMERecord) Equal(other Record) bool {
	o, ok := other.(*CNAMERecord)
	if !ok {
		return false
	}
	return c.DnsRecord.Equal(o.DnsRecord)
}

func isValidHostname(hostname string) bool {
	if len(hostname) > 255 {
		return false
	}
	return hostnameRegexp.MatchString(hostname)
}
