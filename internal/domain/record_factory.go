package domain

import (
	"fmt"
	"strings"
)

func ParseKind(s string) (RecordKind, error) {
	switch strings.ToUpper(s) {
	case "A":
		return RecordA, nil
	case "AAAA":
		return RecordAAAA, nil
	case "CNAME":
		return RecordCNAME, nil
	default:
		return "", fmt.Errorf("unsupported record kind %q", s)
	}
}

func NewFromKind(kind RecordKind, name, value string) (Record, error) {
	switch kind {
	case RecordA:
		return NewA(name, value)
	case RecordAAAA:
		return NewAAAA(name, value)
	case RecordCNAME:
		return NewCNAME(name, value)
	default:
		return Record{}, fmt.Errorf("unsupported record kind %q", kind)
	}
}

// Handy for callers holding a raw string
func NewFromString(kind, name, value string) (Record, error) {
	k, err := ParseKind(kind)
	if err != nil {
		return Record{}, err
	}
	return NewFromKind(k, name, value)
}
