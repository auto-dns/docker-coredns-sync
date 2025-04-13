package core

import (
	"fmt"

	"github.com/StevenC4/docker-coredns-sync/internal/intent"
	"github.com/rs/zerolog"
)

func recordKey(r intent.RecordIntent) string {
	return fmt.Sprintf("%s|%s|%s", r.Record.GetName(), r.Record.GetRecordType(), r.Record.GetValue())
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
func FilterRecordIntents(records []intent.RecordIntent, logger zerolog.Logger) []intent.RecordIntent {
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
		recordType := r.Record.GetRecordType()
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

// ReconcileAndValidate compares desired and actual states and returns two slices:
// - toAdd: desired records that should be added to the registry.
// - toRemove: actual records that should be removed.
func ReconcileAndValidate(desired, actual []intent.RecordIntent, logger zerolog.Logger) (toAdd []intent.RecordIntent, toRemove []intent.RecordIntent) {
	// Build a map key->RecordIntent for desired and actual.
	desiredMap := make(map[string]intent.RecordIntent)
	for _, d := range desired {
		desiredMap[recordKey(d)] = d
	}
	actualMap := make(map[string]intent.RecordIntent)
	for _, a := range actual {
		actualMap[recordKey(a)] = a
	}

	// Any actual record not present in desired is stale: mark for removal.
	for key, act := range actualMap {
		if _, exists := desiredMap[key]; !exists {
			logger.Info().Msgf("[reconciler] Removing stale record: %s (owned by %s/%s)", act.Record.Render(), act.Hostname, act.ContainerName)
			toRemove = append(toRemove, act)
		}
	}

	// For each desired record, decide whether to add it (if it differs from actual).
	for key, des := range desiredMap {
		if act, exists := actualMap[key]; exists {
			// If they are equal, then nothing to do.
			if act.Equal(des) {
				continue
			}
			// Conflict: choose according to business rules.
			if des.Force || des.Created.Before(act.Created) {
				logger.Info().Msgf("[reconciler] Will replace remote record %s with local %s", act.Record.Render(), des.Record.Render())
				toAdd = append(toAdd, des)
				toRemove = append(toRemove, act)
			}
		} else {
			// Not found in actual: add it.
			logger.Info().Msgf("[reconciler] Adding new record: %s (owned by %s/%s)", des.Record.Render(), des.Hostname, des.ContainerName)
			toAdd = append(toAdd, des)
		}
	}
	return toAdd, toRemove
}
