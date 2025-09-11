package core

import (
	"fmt"
	"strings"

	"github.com/auto-dns/docker-coredns-sync/internal/domain"
)

type LabeledRecord struct {
	prefix string
	Kind   domain.RecordKind // A, AAAA, CNAME
	Name   string
	Value  string // may be empty for A (defaults)
	Alias  string // optional
	Force  *bool  // Force is tri-state: nil = not specified on the record label, non-nil = explicit value
}

func (lr LabeledRecord) GetNameLabel() string {
	if lr.Alias == "" {
		return fmt.Sprintf("%s.%s.name", lr.prefix, lr.Kind)
	} else {
		return fmt.Sprintf("%s.%s.%s.name", lr.prefix, lr.Kind, lr.Alias)
	}
}

func (lr LabeledRecord) GetValueLabel() string {
	if lr.Alias == "" {
		return fmt.Sprintf("%s.%s.value", lr.prefix, lr.Kind)
	} else {
		return fmt.Sprintf("%s.%s.%s.value", lr.prefix, lr.Kind, lr.Alias)
	}
}

type ParsedLabels struct {
	Enabled        bool
	ContainerForce bool
	Records        []LabeledRecord
}

func ParseLabels(prefix string, labels map[string]string) ParsedLabels {
	pl := ParsedLabels{}

	// container-level flags
	// -- See if the container is enabled for this app
	if strings.ToLower(labels[prefix+".enabled"]) == "true" {
		pl.Enabled = true
	}
	// -- See if the container is configured to force override
	if v, ok := labels[prefix+".force"]; ok {
		b := strings.ToLower(v) == "true"
		pl.ContainerForce = b
	}

	// aggregate by (recordType|alias)
	type aggregation struct {
		kind  domain.RecordKind
		alias string
		name  string
		value string
		force *bool
	}

	aggregations := map[string]*aggregation{}

	// Loop over remaining labels
	for k, v := range labels {
		// Skip labels not in scope for this app
		if !strings.HasPrefix(k, prefix+".") {
			continue
		}

		parts := strings.Split(k, ".") // coredns.<TYPE>[.<alias>].(name|value|force)
		if len(parts) < 3 {
			continue
		}

		rawType := parts[1]
		kind, err := domain.ParseKind(rawType)
		if err != nil {
			// unknown record kind; skip
			continue
		}

		alias := ""
		keyIdx := 2
		if len(parts) >= 4 {
			alias = parts[2]
			keyIdx = 3
		}
		if keyIdx >= len(parts) {
			continue
		}

		field := parts[keyIdx]
		if field != "name" && field != "value" && field != "force" {
			continue
		}

		labeledRecordKey := string(kind) + "|" + alias
		a, ok := aggregations[labeledRecordKey]
		if !ok {
			a = &aggregation{kind: kind, alias: alias}
			aggregations[labeledRecordKey] = a
		}

		switch field {
		case "name":
			a.name = strings.TrimSpace(v)
		case "value":
			a.value = strings.TrimSpace(v)
		case "force":
			force := boolFromLabel(v)
			a.force = &force
		}
	}

	// flatten to []LabeledRecord
	for _, a := range aggregations {
		pl.Records = append(pl.Records, LabeledRecord{
			prefix: prefix,
			Kind:   a.kind,
			Name:   a.name,
			Value:  a.value, // may be empty; caller decides fallback (e.g., host IP for A)
			Alias:  a.alias,
			Force:  a.force,
		})
	}

	return pl
}

func boolFromLabel(v string) bool { return strings.EqualFold(strings.TrimSpace(v), "true") }
