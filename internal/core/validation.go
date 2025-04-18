package core

import (
	"fmt"

	"github.com/auto-dns/docker-coredns-sync/internal/dns"
	"github.com/auto-dns/docker-coredns-sync/internal/intent"
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

	sameNameARecordsExist := false
	identicalARecordExists := false
	sameNameCNAMERecordsExist := false
	// Forward map used for CNAME cycle detection
	forwardMap := make(map[string]string)
	for _, ri := range existingRecordIntents {
		r := ri.Record
		sameName := r.GetName() == newRecord.GetName()
		if _, ok := r.(*dns.ARecord); ok {
			sameNameARecordsExist = sameNameARecordsExist || sameName
			identicalARecordExists = identicalARecordExists || sameName && r.GetValue() == newRecord.GetValue()
		} else if _, ok := r.(*dns.CNAMERecord); ok {
			sameNameCNAMERecordsExist = sameNameCNAMERecordsExist || sameName
			if _, exists := forwardMap[r.GetName()]; exists {
				logger.Warn().Msg(fmt.Sprintf("Duplicate CNAME definitions detected in remote registry for domain %s", r.GetName()))
			} else {
				forwardMap[r.GetName()] = r.GetValue()
			}
		} else {
			logger.Warn().Msg(fmt.Sprintf("Unknown record type in existing records: %s", ri.Record.GetType()))
		}
	}

	// Rule 1: A and CNAME with same name not allowed
	if isA {
		if sameNameCNAMERecordsExist {
			return NewRecordValidationError(fmt.Sprintf("%s -> %s - cannot add an A record when a CNAME record exists with the same name", newRecord.GetName(), newRecord.GetValue()))
		}
	} else if isCname {
		if sameNameARecordsExist {
			return NewRecordValidationError(fmt.Sprintf("%s -> %s - cannot add a CNAME record when an A record exists with the same name", newRecord.GetName(), newRecord.GetValue()))
		}
	} else {
		return NewRecordValidationError(fmt.Sprintf("%s - unsupported record type", newRecord.GetType()))
	}

	// Rule 2: Multiple CNAMEs with same name not allowed
	if isCname && sameNameCNAMERecordsExist {
		return NewRecordValidationError(fmt.Sprintf("%s -> %s - multiple CNAME records with the same name are not allowed", newRecord.GetName(), newRecord.GetValue()))
	}

	// Rule 3: Multiple A records with the same IP address not allowed
	if isA && identicalARecordExists {
		return NewRecordValidationError(fmt.Sprintf("%s -> %s - multiple A records with the same IP address are not allowed", newRecord.GetName(), newRecord.GetValue()))
	}

	// Rule 4: Detect cycles
	if isCname {
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
