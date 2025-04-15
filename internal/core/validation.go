package core

import (
	"fmt"

	"github.com/StevenC4/docker-coredns-sync/internal/dns"
	"github.com/StevenC4/docker-coredns-sync/internal/intent"
	"github.com/rs/zerolog"
)

func ValidateRecord(newRecordIntent *intent.RecordIntent, existingRecordIntents []*intent.RecordIntent, logger zerolog.Logger) error {
	// Validates a proposed DNS record against the current known records.

	// Rules enforced:
	// 1. A and CNAME records may not coexist for the same name.
	// 2. No duplicate CNAMEs.
	// 3. A records with the same IP are disallowed for the same name.
	// 4. CNAMEs may not form resolution cycles.
	newRecord := newRecordIntent.Record

	_, isCname := newRecord.(*dns.CNAMERecord)
	_, isA := newRecord.(*dns.ARecord)

	var existingRecords []*dns.Record
	var sameNameRecords []*dns.Record
	var aRecords []*dns.Record
	identicalARecordExists := false
	var cnameRecords []*dns.Record
	for _, ri := range existingRecordIntents {
		ri := ri
		existingRecords = append(existingRecords, &ri.Record)
		if ri.Record.GetName() == newRecord.GetName() {
			sameNameRecords = append(sameNameRecords, &ri.Record)
			if isA {
				aRecords = append(aRecords, &ri.Record)
				if ri.Record.GetValue() == newRecord.GetValue() {
					identicalARecordExists = true
				}
			}
			if isCname {
				cnameRecords = append(cnameRecords, &ri.Record)
			}
		}
	}

	// Rule 1: A and CNAME with same name not allowed
	if isA {
		if len(cnameRecords) > 0 {
			return NewRecordValidationError(fmt.Sprintf("%s -> %s - cannot add an A record when a CNAME record exists with the same name", newRecord.GetName(), newRecord.GetValue()))
		}
	} else if isCname {
		if len(aRecords) > 0 {
			return NewRecordValidationError(fmt.Sprintf("%s -> %s - cannot add a CNAME record when an A record exists with the same name", newRecord.GetName(), newRecord.GetValue()))
		}
	} else {
		return NewRecordValidationError(fmt.Sprintf("%s - unsupported record type", newRecord.GetType()))
	}

	// Rule 2: Multiple CNAMEs with same name not allowed
	if isCname && len(cnameRecords) > 0 {
		return NewRecordValidationError(fmt.Sprintf("%s -> %s - multiple CNAME records with the same name are not allowed", newRecord.GetName(), newRecord.GetValue()))
	}

	// Rule 3: Multiple A records with the same IP address not allowed
	if isA && identicalARecordExists {
		return NewRecordValidationError(fmt.Sprintf("%s -> %s - multiple A records with the same IP address are not allowed", newRecord.GetName(), newRecord.GetValue()))
	}

	// Rule 4: Detect cycles
	if isCname {
		forwardMap := make(map[string]string)
		for _, r := range existingRecords {
			r := r
			if _, ok := (*r).(*dns.CNAMERecord); ok {
				if _, exists := forwardMap[(*r).GetName()]; exists {
					logger.Warn().Msg(fmt.Sprintf("Duplicate CNAME definitions detected in remote registry for domain %s", (*r).GetName()))
					continue
				}
			}
			forwardMap[(*r).GetName()] = (*r).GetValue()
		}
		forwardMap[newRecord.GetName()] = newRecord.GetValue()

		// Process to detect loops
		seenMap := make(map[string]struct{})
		name := newRecord.GetName()
		for {
			value, exists := forwardMap[name]
			if !exists {
				break
			}
			if _, seen := seenMap[name]; seen {
				return NewRecordValidationError(fmt.Sprintf("CNAME cycle detected starting at: %s", newRecord.GetName()))
			}
			seenMap[name] = struct{}{}
			name = value
		}
	}
	return nil
}
