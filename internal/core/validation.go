package core

import (
	"fmt"

	"github.com/auto-dns/docker-coredns-sync/internal/domain"
	"github.com/rs/zerolog"
)

func ValidateRecord(newRecordIntent *domain.RecordIntent, existingRecordIntents []*domain.RecordIntent, logger zerolog.Logger) error {
	// Validates a proposed DNS record against the current known records.

	// Rules enforced:
	// 1. A and CNAME records may not coexist for the same name.
	// 2. No duplicate CNAMEs.
	// 3. A records with the same IP are disallowed for the same name.
	// 4. CNAMEs may not form resolution cycles.
	newRecord := newRecordIntent.Record

	sameNameARecordsExist := false
	identicalARecordExists := false
	sameNameCNAMERecordsExist := false
	// Forward map used for CNAME cycle detection
	forwardMap := make(map[string]string)
	for _, ri := range existingRecordIntents {
		r := ri.Record
		sameName := r.Name == newRecord.Name
		if r.IsA() {
			sameNameARecordsExist = sameNameARecordsExist || sameName
			identicalARecordExists = identicalARecordExists || sameName && r.Value == newRecord.Value
		} else if r.IsCNAME() {
			sameNameCNAMERecordsExist = sameNameCNAMERecordsExist || sameName
			if _, exists := forwardMap[r.Name]; exists {
				logger.Warn().Msg(fmt.Sprintf("Duplicate CNAME definitions detected in remote registry for domain %s", r.Name))
			} else {
				forwardMap[r.Name] = r.Value
			}
		} else {
			logger.Warn().Msg(fmt.Sprintf("Unknown record type in existing records: %s", ri.Record.Type))
		}
	}

	// Rule 1: A and CNAME with same name not allowed
	if newRecord.IsA() {
		if sameNameCNAMERecordsExist {
			return NewRecordValidationError(fmt.Sprintf("%s -> %s - cannot add an A record when a CNAME record exists with the same name", newRecord.Name, newRecord.Value))
		}
	} else if newRecord.IsCNAME() {
		if sameNameARecordsExist {
			return NewRecordValidationError(fmt.Sprintf("%s -> %s - cannot add a CNAME record when an A record exists with the same name", newRecord.Name, newRecord.Value))
		}
	} else {
		return NewRecordValidationError(fmt.Sprintf("%s - unsupported record type", newRecord.Type))
	}

	// Rule 2: Multiple CNAMEs with same name not allowed
	if newRecord.IsCNAME() && sameNameCNAMERecordsExist {
		return NewRecordValidationError(fmt.Sprintf("%s -> %s - multiple CNAME records with the same name are not allowed", newRecord.Name, newRecord.Value))
	}

	// Rule 3: Multiple A records with the same IP address not allowed
	if newRecord.IsA() && identicalARecordExists {
		return NewRecordValidationError(fmt.Sprintf("%s -> %s - multiple A records with the same IP address are not allowed", newRecord.Name, newRecord.Value))
	}

	// Rule 4: Detect cycles
	if newRecord.IsCNAME() {
		forwardMap[newRecord.Name] = newRecord.Value

		// Process to detect loops
		seenMap := make(map[string]struct{})
		name := newRecord.Name
		for {
			value, exists := forwardMap[name]
			if !exists {
				break
			}
			if _, seen := seenMap[name]; seen {
				return NewRecordValidationError(fmt.Sprintf("CNAME cycle detected starting at: %s", newRecord.Name))
			}
			seenMap[name] = struct{}{}
			name = value
		}
	}
	return nil
}
