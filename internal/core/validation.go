package core

import (
	"fmt"

	"github.com/auto-dns/docker-coredns-sync/internal/domain"
	"github.com/rs/zerolog"
)

func ValidateRecord(newRI *domain.RecordIntent, existing []*domain.RecordIntent, logger zerolog.Logger) error {
	// Validates a proposed DNS record against the current known records.

	// Rules enforced:
	// 1. A/AAAA and CNAME records may not coexist for the same name.
	// 2. No duplicate CNAMEs.
	// 3. A/AAAA records with the same address are disallowed for the same name+family.
	// 4. CNAMEs may not form resolution cycles.
	newR := newRI.Record

	// presence flags + duplicate checks per family
	var sameNameCNAME bool
	var sameNameAnyAddress bool
	var dupAValue, dupAAAAValue bool

	for _, ri := range existing {
		r := ri.Record
		sameName := r.Name == newR.Name
		sameValue := r.Value == newR.Value
		switch {
		case r.IsA():
			if sameName {
				sameNameAnyAddress = true
			}
			if sameName && sameValue && newR.IsA() {
				dupAValue = true
			}
		case r.IsAAAA():
			if sameName {
				sameNameAnyAddress = true
			}
			if sameName && sameValue && newR.IsAAAA() {
				dupAAAAValue = true
			}
		case r.IsCNAME():
			if sameName {
				sameNameCNAME = true
			}
		default:
			logger.Warn().Str("kind", string(r.Kind)).Msg(fmt.Sprintf("Unknown record kind in existing records"))
		}
	}

	// Rule 1: CNAME cannot coexist any address records (A or AAAA)
	if newR.IsCNAME() && sameNameAnyAddress {
		return NewRecordValidationError(fmt.Sprintf("%s -> %s - cannot add a CNAME when A/AAAA records exist with the same name", newR.Name, newR.Value))
	}
	if (newR.IsAddress()) && sameNameCNAME {
		return NewRecordValidationError(fmt.Sprintf("%s -> %s - cannot add A/AAAA when a CNAME exists with the same name", newR.Name, newR.Value))
	}

	// Rule 2: Only one CNAME per name
	if newR.IsCNAME() && sameNameCNAME {
		return NewRecordValidationError(fmt.Sprintf("%s -> %s - multiple CNAME records with the same name are not allowed", newR.Name, newR.Value))
	}

	// Rule 3: no duplicate address values per family
	if newR.IsA() && dupAValue {
		return NewRecordValidationError(fmt.Sprintf("%s -> %s - duplicate A record value not allowed", newR.Name, newR.Value))
	}
	if newR.IsAAAA() && dupAAAAValue {
		return NewRecordValidationError(fmt.Sprintf("%s -> %s - duplicate AAAA record value not allowed", newR.Name, newR.Value))
	}

	// Rule 4: CNAME cycle detection
	if newR.IsCNAME() {
		forward := map[string]string{}
		for _, ri := range existing {
			if ri.Record.IsCNAME() {
				if _, exists := forward[ri.Record.Name]; exists {
					logger.Warn().Msg(fmt.Sprintf("Duplicate CNAME definitions detected in remote registry for domain %s", ri.Record.Name))
				} else {
					forward[ri.Record.Name] = ri.Record.Value
				}
			}
		}
		forward[newR.Name] = newR.Value

		seen := map[string]struct{}{}
		cur := newR.Name
		for {
			v, ok := forward[cur]
			if !ok {
				break
			}
			if _, s := seen[cur]; s {
				return NewRecordValidationError(fmt.Sprintf("CNAME cycle detected starting at: %s", newR.Name))
			}
			seen[cur] = struct{}{}
			cur = v
		}
	}

	return nil
}
