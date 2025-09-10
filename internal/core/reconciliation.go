package core

import (
	"github.com/auto-dns/docker-coredns-sync/internal/config"
	"github.com/auto-dns/docker-coredns-sync/internal/domain"

	"github.com/rs/zerolog"
)

func shouldReplaceExisting(new, existing *domain.RecordIntent, logger zerolog.Logger) bool {
	l := logger.With().Str("reconciler", "filter").Str("new_record", new.Render()).Str("existing_record", existing.Render()).Logger()
	if new.Force && !existing.Force {
		l.Trace().Msg("Replacing existing record due to force label on new record")
		return true
	} else if !new.Force && existing.Force {
		l.Trace().Msg("Keeping existing record due to force label on existing record")
		return false
	} else if new.Created.Before(existing.Created) {
		l.Trace().Msg("Replacing existing record due to the new record's container being older")
		return true
	}
	l.Trace().Msg("Keeping existing record due to the existing record's container being older")
	return false
}

func shouldReplaceAllExisting(new *domain.RecordIntent, existing []*domain.RecordIntent, logger zerolog.Logger) bool {
	// Returns True if `new` (CNAME) should replace all `existing` (A records).

	// Rules:
	// - If any existing is force and new is not, new loses.
	// - If new is force and all existing are not, new wins.
	// - If mixed force values exist and new is force:
	//     - New must be older than *all* existing force records.
	//     - Otherwise, new loses.
	// - If force flags match for all (either all force or all non-force), the oldest record wins.
	existingRecordStrings := make([]string, 0, len(existing))
	for _, ri := range existing {
		existingRecordStrings = append(existingRecordStrings, ri.Render())
	}
	l := logger.With().Str("reconciler", "filter").Str("new_record", new.Render()).Strs("existing_records", existingRecordStrings).Logger()
	if len(existing) == 0 {
		return true
	}

	anyForce := false
	allForce := true
	allNonForce := true
	newCreatedBeforeAllOldWithForce := true
	newCreatedBeforeAll := true
	for _, ri := range existing {
		newCreatedBeforeExisting := new.Created.Before(ri.Created)
		if ri.Force {
			anyForce = true
			allNonForce = false
			if !newCreatedBeforeExisting {
				newCreatedBeforeAllOldWithForce = false
			}
		} else {
			allForce = false
		}
		if !newCreatedBeforeExisting {
			newCreatedBeforeAll = false
		}
	}

	// If any existing is force and new is not, new loses.
	if anyForce && !new.Force {
		l.Trace().Msg("Keeping all existing records because one of their containers has the force label and the new record's container does not")
		return false
	}

	// If new is force and all existing are not, new wins.
	if new.Force && allNonForce {
		l.Trace().Msg("Replacing all existing records with the new one because none of the existing record's containers has the force label and the new record's container does")
		return true
	}

	// If mixed force values exist and new is force:
	// New must be older than all existing force records.
	if new.Force && !allForce {
		if newCreatedBeforeAllOldWithForce {
			l.Trace().Msg("Replacing all existing new records with the new one because the new record's container has the force label and was created before all of the existing records' containers that have the force label")
			return true
		}
		l.Trace().Msg("Keeping all existing new records - although the new one's container has the force label, one or more of the existing records' containers with the force label was created before it")
		return false
	}

	// Otherwise, when force flags match (either all true or all false), the oldest wins.
	if newCreatedBeforeAll {
		l.Trace().Msg("Replacing all existing records with the new one because none of the containers have the force label and the new record's container is older than the containers of all the existing records")
		return true
	}
	l.Trace().Msg("Keeping all existing records because none of the containers have the force label and the new record's container is not older than the containers of all the existing records")
	return false
}

// FilterRecordIntents receives a slice of RecordIntent (desired) and filters out conflicting ones.
// It returns a reconciled slice of RecordIntent.
func FilterRecordIntents(recordIntents []*domain.RecordIntent, logger zerolog.Logger) []*domain.RecordIntent {
	logger.Debug().Msg("Reconciling desired records against each other")

	desiredByNameType := newNestedRecordMap()
	for _, ri := range recordIntents {
		if ri.Record.IsA() {
			existingARecordIntent, duplicateExists := desiredByNameType.PeekNameTypeRecord(ri.Record.Name, "A", ri.Record.Value)
			if !duplicateExists || shouldReplaceExisting(ri, existingARecordIntent, logger) {
				desiredByNameType.Get(ri.Record.Name).Get("A").Set(ri.Record.Value, ri)
			}
		} else if ri.Record.IsCNAME() {
			if existingCNAMERecordIntents, exists := desiredByNameType.PeekNameTypeRecords(ri.Record.Name, "CNAME"); exists {
				// Get existing CNAME record - assume only one
				existingCNAMERecordIntent := existingCNAMERecordIntents[0]
				if shouldReplaceExisting(ri, existingCNAMERecordIntent, logger) {
					// Replace CNAME record
					// Just calling the "Set" function isn't enough to prevent multiple CNAME records with different values
					desiredByNameType.Get(ri.Record.Name).Get("CNAME").Delete(existingCNAMERecordIntent.Record.Value)
					desiredByNameType.Get(ri.Record.Name).Get("CNAME").Set(ri.Record.Value, ri)
				}
			} else {
				// No conflict - just add it
				desiredByNameType.Get(ri.Record.Name).Get("CNAME").Set(ri.Record.Value, ri)
			}
		}
	}

	desiredByNameTypeDeduplicated := newNestedRecordMap()
	for _, name := range desiredByNameType.GetAllNames() {
		aRecords, aRecordsExist := desiredByNameType.PeekNameTypeRecords(name, "A")
		cnameRecords, cnameRecordsExist := desiredByNameType.PeekNameTypeRecords(name, "CNAME")
		if aRecordsExist && !cnameRecordsExist {
			// Transfer all A records into the "desired by name type deduplicated" set
			for _, ri := range aRecords {
				desiredByNameTypeDeduplicated.Get(name).Get("A").Set(ri.Record.Value, ri)
			}
		} else if cnameRecordsExist && !aRecordsExist {
			// Transfer the CNAME record into the "desired by name type deduplicated" set
			ri := cnameRecords[0]
			desiredByNameTypeDeduplicated.Get(name).Get("CNAME").Set(ri.Record.Value, ri)
		} else if aRecordsExist && cnameRecordsExist {
			cnameRecord := cnameRecords[0]
			if shouldReplaceAllExisting(cnameRecord, aRecords, logger) {
				desiredByNameTypeDeduplicated.Get(name).Get("CNAME").Set(cnameRecord.Record.Value, cnameRecord)
			} else {
				for _, ri := range aRecords {
					desiredByNameTypeDeduplicated.Get(name).Get("A").Set(ri.Record.Value, ri)
				}
			}
		} else {
			logger.Warn().Str("name", name).Msg("Found a record name with no CNAME or A records. Skipping it.")
		}
	}

	return desiredByNameTypeDeduplicated.GetAllValues()
}

func ReconcileAndValidate(desired, actual []*domain.RecordIntent, cfg *config.AppConfig, logger zerolog.Logger) ([]*domain.RecordIntent, []*domain.RecordIntent) {
	toAddMap := map[string]*domain.RecordIntent{}
	toRemoveMap := map[string]*domain.RecordIntent{}

	actualByNameType := newNestedRecordMap()
	desiredSet := make(map[string]struct{}, len(desired))
	for _, ri := range desired {
		desiredSet[ri.Key()] = struct{}{}
	}

	// Step 1: Remove stale records and build lookup structure
	for _, ri := range actual {
		if _, exists := desiredSet[ri.Key()]; !exists {
			if ri.Hostname == cfg.Hostname {
				logger.Info().Msgf("Removing stale record: %s (owned by %s/%s)",
					ri.Record.Render(), ri.Hostname, ri.ContainerName)
				toRemoveMap[ri.Key()] = ri
			} else {
				logger.Debug().Msgf("Skipping removal of record %s not owned by this host (%s != %s)",
					ri.Record.Render(), ri.Hostname, cfg.Hostname)
			}
		} else {
			actualByNameType.Get(ri.Record.Name).Get(ri.Record.Type).Set(ri.Record.Value, ri)
		}
	}

	// Step 2: Reconcile each desired record
	for _, desiredRecordIntent := range desired {
		evictions := map[string]*domain.RecordIntent{}

		if desiredRecordIntent.Record.IsA() {
			if actualRecordIntents, exists := actualByNameType.PeekNameTypeRecords(desiredRecordIntent.Record.Name, "CNAME"); exists {
				// Conflict: desired A, actual has CNAME(s)
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
			} else if actualRecordIntent, exists := actualByNameType.PeekNameTypeRecord(desiredRecordIntent.Record.Name, "A", desiredRecordIntent.Record.Value); exists {
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
		} else if desiredRecordIntent.Record.IsCNAME() {
			if actualRecordIntents, exists := actualByNameType.PeekNameTypeRecords(desiredRecordIntent.Record.Name, "A"); exists {
				desiredOlderThanAllActual := true
				for _, ri := range actualRecordIntents {
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
						actualAStrings[i] = ri.Render()
						evictions[ri.Key()] = ri
					}
					logger.Warn().Strs("actual_record_intents", actualAStrings).Str("desired_record_intent", desiredRecordIntent.Render()).Msg("Record conflict between local and remote - evicting remote due to container age")
				} else {
					continue
				}
			} else if actualRecordIntents, exists := actualByNameType.PeekNameTypeRecords(desiredRecordIntent.Record.Name, "CNAME"); exists {
				actualRecordIntent := actualRecordIntents[0]
				if actualRecordIntent.Equal(*desiredRecordIntent) {
					// Skip - we don't need to replace ourselves
					continue
				} else if desiredRecordIntent.Force {
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
					continue
				}
			}
			// Else: don't skip - just add with no evictions - no need for an else statement, this will just work
		}

		// Step 3: Simulate state for validation
		keysToRemove := make(map[string]struct{})
		for key := range toRemoveMap {
			keysToRemove[key] = struct{}{}
		}
		for key := range evictions {
			keysToRemove[key] = struct{}{}
		}
		var simulated []*domain.RecordIntent
		for _, ri := range actual {
			if _, exists := keysToRemove[ri.Key()]; !exists {
				simulated = append(simulated, ri)
			}
		}

		// Step 4: Validate and commit
		if err := ValidateRecord(desiredRecordIntent, simulated, logger); err == nil {
			logger.Info().Msgf("Adding new record: %s", desiredRecordIntent.Render())
			toAddMap[desiredRecordIntent.Record.Key()] = desiredRecordIntent
			for k, v := range evictions {
				toRemoveMap[k] = v
			}
		} else {
			logger.Warn().Err(err).Msgf("Skipping invalid record %s", desiredRecordIntent.Record.Render())
		}
	}

	// Step 5: Convert maps to slices
	var toAdd, toRemove []*domain.RecordIntent
	for _, r := range toAddMap {
		toAdd = append(toAdd, r)
	}
	for _, r := range toRemoveMap {
		toRemove = append(toRemove, r)
	}
	return toAdd, toRemove
}
