package domain

import (
	"fmt"
	"net"
	"regexp"
)

type RecordKind string

const (
	RecordA     RecordKind = "A"
	RecordAAAA  RecordKind = "AAAA"
	RecordCNAME RecordKind = "CNAME"
)

type Record struct {
	Name  string
	Type  RecordKind
	Value string
}

func NewA(name, ipv4 string) (Record, error) {
	if !isValidHostname(name) {
		return Record{}, fmt.Errorf("invalid A name: %s", name)
	}

	ip := net.ParseIP(ipv4)
	if ip == nil || ip.To4() == nil {
		return Record{}, fmt.Errorf("invalid IPv4: %s", ipv4)
	}

	return Record{
		Name:  name,
		Type:  RecordA,
		Value: ipv4,
	}, nil
}

func NewAAAA(name, ipv6 string) (Record, error) {
	if !isValidHostname(name) {
		return Record{}, fmt.Errorf("invalid AAAA name: %s", name)
	}

	ip := net.ParseIP(ipv6)
	if ip == nil || ip.To16() == nil || ip.To4() != nil {
		return Record{}, fmt.Errorf("invalid IPv6: %s", ipv6)
	}

	return Record{
		Name:  name,
		Type:  RecordAAAA,
		Value: ipv6,
	}, nil
}

func NewCNAME(name, target string) (Record, error) {
	if !isValidHostname(name) || !isValidHostname(target) {
		return Record{}, fmt.Errorf("invalid CNAME: %s -> %s", name, target)
	}

	return Record{
		Name:  name,
		Type:  RecordCNAME,
		Value: target,
	}, nil
}

func (r Record) Key() string {
	return fmt.Sprintf("%s|%s|%s", r.Name, r.Type, r.Value)
}

func (r Record) Render() string {
	if r.Value == "" {
		return fmt.Sprintf("[%s] %s -> <no value>", r.Type, r.Name)
	}
	return fmt.Sprintf("[%s] %s -> %s", r.Type, r.Name, r.Value)
}

func (r Record) Equal(o Record) bool {
	return r.Name == o.Name && r.Type == o.Type && r.Value == o.Value
}

var hostnameRegexp = regexp.MustCompile(`^[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(?:\.[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*$`)

func isValidHostname(h string) bool {
	return len(h) > 0 && len(h) <= 255 && hostnameRegexp.MatchString(h)
}

func (r Record) IsA() bool       { return r.Type == RecordA }
func (r Record) IsAAAA() bool    { return r.Type == RecordAAAA }
func (r Record) IsCNAME() bool   { return r.Type == RecordCNAME }
func (r Record) IsAddress() bool { return r.Type == RecordA || r.Type == RecordAAAA }
