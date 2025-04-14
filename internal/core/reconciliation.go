package core

import (
	"github.com/StevenC4/docker-coredns-sync/internal/intent"
	"github.com/StevenC4/docker-coredns-sync/internal/util"
	"github.com/rs/zerolog"
)

type DMap[K comparable, V any] = *util.DefaultMap[K, V]
type RecordMap = DMap[string, *intent.RecordIntent]
type TypeMap = DMap[string, RecordMap]
type DomainMap = DMap[string, TypeMap]

func NewNestedRecordMap() DomainMap {
	return util.NewDefaultMap[string](
		func() TypeMap {
			return util.NewDefaultMap[string](
				func() RecordMap {
					return util.NewDefaultMap[string](
						func() *intent.RecordIntent { return nil },
					)
				},
			)
		},
	)
}

func shouldReplaceExisting(new, existing intent.RecordIntent) bool {
	if new.Force && !existing.Force {
		return true
	}
	if !new.Force && existing.Force {
		return false
	}
	return new.Created.Before(existing.Created)
}

func shouldReplaceAllExisting(new intent.RecordIntent, existing []intent.RecordIntent) bool {
	// Returns True if `new` (CNAME) should replace all `existing` (A records).

	// Rules:
	// - If any existing is force and new is not, new loses.
	// - If new is force and all existing are not, new wins.
	// - If mixed force values exist and new is force:
	//     - New must be older than *all* existing force records.
	//     - Otherwise, new loses.
	// - If force flags match for all (either all force or all non-force), the oldest record wins.
	if len(existing) == 0 {
		return true
	}

	anyForce := false
	allForce := true
	allNonForce := true
	for _, r := range existing {
		if r.Force {
			anyForce = true
		} else {
			allForce = false
		}
		if r.Force {
			allNonForce = false
		}
	}

	// If any existing is force and new is not, new loses.
	if anyForce && !new.Force {
		return false
	}

	// If new is force and all existing are not, new wins.
	if new.Force && allNonForce {
		return true
	}

	// If mixed force values exist and new is force:
	// New must be older than all existing force records.
	if new.Force && !allForce {
		for _, r := range existing {
			if r.Force && !new.Created.Before(r.Created) {
				return false
			}
		}
		return true
	}

	// Otherwise, when force flags match (either all true or all false), the oldest wins.
	for _, r := range existing {
		if !new.Created.Before(r.Created) {
			return false
		}
	}
	return true
}

// FilterRecordIntents receives a slice of RecordIntent (desired) and filters out conflicting ones.
// It returns a reconciled slice of RecordIntent.
func FilterRecordIntents(records []*intent.RecordIntent, logger zerolog.Logger) []intent.RecordIntent {
	// We'll build a nested map:
	// map[recordName]map[recordType]map[recordValue]RecordIntent
	desiredByNameType := make(map[string]map[string]map[string]intent.RecordIntent)

	// Helper: ensure nested maps exist.
	ensureEntry := func(name, rType string) {
		if _, exists := desiredByNameType[name]; !exists {
			desiredByNameType[name] = make(map[string]map[string]intent.RecordIntent)
		}
		if _, exists := desiredByNameType[name][rType]; !exists {
			desiredByNameType[name][rType] = make(map[string]intent.RecordIntent)
		}
	}

	for _, r := range records {
		name := r.Record.GetName()
		value := r.Record.GetValue()

		// Ensure there is a map for this record name and type.
		recordType := r.Record.GetType()
		ensureEntry(name, recordType)

		// Check for conflicts between A and CNAME record types:
		// We want to enforce: if an A record exists, and a CNAME comes in for same name, we choose one based on business rules.
		if recordType == "A" {
			if other, exists := desiredByNameType[name]["CNAME"]; exists && len(other) > 0 {
				// Get an existing CNAME record (assume only one exists)
				var existing intent.RecordIntent
				for _, v := range other {
					existing = v
					break
				}
				if shouldReplaceExisting(r, existing) {
					// Remove CNAME records.
					delete(desiredByNameType[name], "CNAME")
					ensureEntry(name, "A")
					desiredByNameType[name]["A"][value] = r
				}
				continue
			}
			// For A records: if already exists with same value, check which wins.
			if existing, exists := desiredByNameType[name]["A"][value]; exists {
				if shouldReplaceExisting(r, existing) {
					desiredByNameType[name]["A"][value] = r
				}
			} else {
				desiredByNameType[name]["A"][value] = r
			}
		} else if recordType == "CNAME" {
			if others, exists := desiredByNameType[name]["A"]; exists && len(others) > 0 {
				// Conflict with existing A records.
				allA := make([]intent.RecordIntent, 0, len(others))
				for _, rec := range others {
					rec := rec
					allA = append(allA, rec)
				}
				if shouldReplaceAllExisting(r, allA) {
					delete(desiredByNameType[name], "A")
					ensureEntry(name, "CNAME")
					desiredByNameType[name]["CNAME"][value] = r
				}
				continue
			}
			if others, exists := desiredByNameType[name]["CNAME"]; exists && len(others) > 0 {
				// Replace existing CNAME with the new one if it should replace.
				var existing intent.RecordIntent
				for _, rec := range others {
					existing = rec
					break
				}
				if shouldReplaceExisting(r, existing) {
					desiredByNameType[name]["CNAME"][value] = r
				}
			} else {
				ensureEntry(name, "CNAME")
				desiredByNameType[name]["CNAME"][value] = r
			}
		}
	}

	// Flatten nested maps to a slice.
	var reconciled []intent.RecordIntent
	for _, typeMap := range desiredByNameType {
		for _, valueMap := range typeMap {
			for _, intent := range valueMap {
				reconciled = append(reconciled, intent)
			}
		}
	}
	return reconciled
}

func ReconcileAndValidate(desired, actual []*intent.RecordIntent, logger zerolog.Logger) ([]*intent.RecordIntent, []*intent.RecordIntent) {
	toAddMap := map[string]*intent.RecordIntent{}
	toRemoveMap := map[string]*intent.RecordIntent{}

	actualByNameType := NewNestedRecordMap()
	desiredSet := make(map[string]struct{}, len(desired))
	for _, ri := range desired {
		desiredSet[ri.Key()] = struct{}{}
	}

	// Step 1: Remove stale records and build lookup structure
	for _, ri := range actual {
		if _, exists := desiredSet[ri.Key()]; !exists {
			logger.Info().Msgf("[reconciler] Removing stale record: %s (owned by %s/%s)",
				ri.Record.Render(), ri.Hostname, ri.ContainerName)
			toRemoveMap[ri.Key()] = ri
		} else {
			name := ri.Record.GetName()
			recordType := ri.Record.GetType()
			value := ri.Record.GetValue()
			actualByNameType.Get(name).Get(recordType).Set(value, ri)
		}
	}

	// Step 2: Reconcile each desired record
	for _, desiredRecordIntent := range desired {
		name := desiredRecordIntent.Record.GetName()
		value := desiredRecordIntent.Record.GetValue()

		actualAs := actualByNameType.Get(name).Get("A")
		actualCNAMEs := actualByNameType.Get(name).Get("CNAME")

		evictions := map[string]*intent.RecordIntent{}

		switch desiredRecordIntent.Record.GetType() {
		case "A":
			if len(actualCNAMEs.Items()) > 0 {
				// Conflict: desired A, actual has CNAME(s)
				actualRecordIntents := make([]*intent.RecordIntent, 0, len(actualCNAMEs.Items()))
				for _, ri := range actualCNAMEs.Items() {
					actualRecordIntents = append(actualRecordIntents, ri)
				}

				actualRecordIntent := actualRecordIntents[0]
				if desiredRecordIntent.Force {
					actualCnameStrings := make([]string, len(actualRecordIntents))
					for i, ri := range actualRecordIntents {
						actualCnameStrings[i] = ri.Render()
						evictions[ri.Key()] = ri
					}
					logger.Warn().Strs("actual_record_intents", actualCnameStrings).Str("desired_record_intent", desiredRecordIntent.Render()).Msg("Record conflict between local and remote - evicting remote due to force container label")
				} else if desiredRecordIntent.Created.Before(actualRecordIntent.Created) {
					actualCnameStrings := make([]string, len(actualRecordIntents))
					for i, ri := range actualRecordIntents {
						actualCnameStrings[i] = ri.Render()
						evictions[ri.Key()] = ri
					}
					logger.Warn().Strs("actual_record_intents", actualCnameStrings).Str("desired_record_intent", desiredRecordIntent.Render()).Msg("Record conflict between local and remote - evicting remote due to container age")
				} else {
					// We're not evicting, so skip the rest for this record
					continue
				}
			} else if actualRecordIntent := actualAs.Get(value); actualRecordIntent != nil {
				if actualRecordIntent.Equal(*desiredRecordIntent) {
					// Skip - we don't need to replace ourselves
					continue
				} else if desiredRecordIntent.Force {
					logger.Warn().Str("actual_record_intent", actualRecordIntent.Render()).Str("desired_record_intent", desiredRecordIntent.Render()).Msg("Record conflict between local and remote - evicting remote due to force container label")
					evictions[actualRecordIntent.Key()] = actualRecordIntent
				} else if desiredRecordIntent.Created.Before(actualRecordIntent.Created) {
					logger.Warn().Str("actual_record_intent", actualRecordIntent.Render()).Str("desired_record_intent", desiredRecordIntent.Render()).Msg("Record conflict between local and remote - evicting remote due to container age")
					evictions[actualRecordIntent.Key()] = actualRecordIntent
				} else {
					// We're not evicting, so skip the rest for this record
					continue
				}
			}
			// Else: don't skip - just add with no evictions - no need for an else statement, this will just work

		case "CNAME":
			if len(actualAs.Items()) > 0 {
				actualRecordIntents := make([]*intent.RecordIntent, 0, len(actualAs.Items()))
				desiredOlderThanAllActual := true
				for _, ri := range actualAs.Items() {
					actualRecordIntents = append(actualRecordIntents, ri)
					desiredOlderThanAllActual = desiredOlderThanAllActual && desiredRecordIntent.Created.Before(ri.Created)
				}

				if desiredRecordIntent.Force {
					actualAStrings := make([]string, len(actualRecordIntents))
					for i, ri := range actualRecordIntents {
						actualAStrings[i] = ri.Render()
						evictions[ri.Key()] = ri
					}
					logger.Warn().Strs("actual_record_intents", actualAStrings).Str("desired_record_intent", desiredRecordIntent.Render()).Msg("Record conflict between local and remote - evicting remote due to force container label")
				} else if desiredOlderThanAllActual {
					actualAStrings := make([]string, len(actualRecordIntents))
					for i, ri := range actualRecordIntents {
						ri := ri
						actualAStrings[i] = ri.Render()
						evictions[ri.Key()] = ri
					}
					logger.Warn().Strs("actual_record_intents", actualAStrings).Str("desired_record_intent", desiredRecordIntent.Render()).Msg("Record conflict between local and remote - evicting remote due to container age")
				} else {
					continue
				}
			} else if len(actualCNAMEs.Items()) > 0 {
				actualRecordIntents := make([]*intent.RecordIntent, 0, len(actualCNAMEs.Items()))
				for _, ri := range actualCNAMEs.Items() {
					actualRecordIntents = append(actualRecordIntents, ri)
				}

				actualRecordIntent := actualRecordIntents[0]
				if actualRecordIntent.Equal(*desiredRecordIntent) {
					// Skip - we don't need to replace ourselves
					continue
				} else if desiredRecordIntent.Force {
					actualCnameStrings := make([]string, len(actualRecordIntents))
					for i, ri := range actualRecordIntents {
						ri := ri
						actualCnameStrings[i] = ri.Render()
						evictions[ri.Key()] = ri
					}
					logger.Warn().Strs("actual_record_intents", actualCnameStrings).Str("desired_record_intent", desiredRecordIntent.Render()).Msg("Record conflict between local and remote - evicting remote due to force container label")
				} else if desiredRecordIntent.Created.Before(actualRecordIntent.Created) {
					actualCnameStrings := make([]string, len(actualRecordIntents))
					for i, ri := range actualRecordIntents {
						ri := ri
						actualCnameStrings[i] = ri.Render()
						evictions[ri.Key()] = ri
					}
					logger.Warn().Strs("actual_record_intents", actualCnameStrings).Str("desired_record_intent", desiredRecordIntent.Render()).Msg("Record conflict between local and remote - evicting remote due to container age")
				} else {
					continue
				}
			}
			// Else: don't skip - just add with no evictions - no need for an else statement, this will just work
		}

		// Step 3: Simulate state for validation
		var simulated []*intent.RecordIntent
		// Step 3.1: Calculate the "keys to remove"
		keysToRemove := make(map[string]struct{})
		// * to_remove set
		for key := range toRemoveMap {
			keysToRemove[key] = struct{}{}
		}
		// * evictions set
		for key := range evictions {
			keysToRemove[key] = struct{}{}
		}
		// Step 3.2: Filter "actual" into "simulated"
		for _, ri := range actual {
			ri := ri
			if _, exists := keysToRemove[ri.Key()]; !exists {
				simulated = append(simulated, ri)
			}
		}

		// Step 4: Validate and commit
		if err := ValidateRecord(desiredRecordIntent, simulated); err == nil {
			logger.Info().Msgf("[reconciler] Adding new record: %s (owned by %s/%s)",
				desiredRecord.Record.Render(), desiredRecord.Hostname, desiredRecord.ContainerName)
			toAddMap[desiredRecord.Key()] = desiredRecord
			for k, v := range evictions {
				toRemoveMap[k] = v
			}
		} else {
			logger.Warn().Msgf("[reconciler] Skipping invalid record %s — %s", desiredRecord.Record.Render(), err)
		}
	}

	// Step 5: Convert maps to slices
	var toAdd, toRemove []*intent.RecordIntent
	for _, r := range toAddMap {
		toAdd = append(toAdd, r)
	}
	for _, r := range toRemoveMap {
		toRemove = append(toRemove, r)
	}
	return toAdd, toRemove
}
