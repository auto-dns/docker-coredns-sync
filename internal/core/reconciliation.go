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
	// Returns True if `new` (CNAME) should replace all `existing` (A and AAAA records).

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

	desiredByNameKind := newNestedRecordMap()

	// 1. Deduplicate per name+kind using shouldReplaceExisting
	for _, ri := range recordIntents {
		switch {
		case ri.Record.IsA():
			if existing, dup := desiredByNameKind.PeekNameKindRecord(ri.Record.Name, domain.RecordA, ri.Record.Value); !dup || shouldReplaceExisting(ri, existing, logger) {
				desiredByNameKind.Get(ri.Record.Name).Get(domain.RecordA).Set(ri.Record.Value, ri)
			}
		case ri.Record.IsAAAA():
			if existing, dup := desiredByNameKind.PeekNameKindRecord(ri.Record.Name, domain.RecordAAAA, ri.Record.Value); !dup || shouldReplaceExisting(ri, existing, logger) {
				desiredByNameKind.Get(ri.Record.Name).Get(domain.RecordAAAA).Set(ri.Record.Value, ri)
			}
		case ri.Record.IsCNAME():
			if cnames, exists := desiredByNameKind.PeekNameKindRecords(ri.Record.Name, domain.RecordCNAME); exists {
				// Get existing CNAME record - assume only one
				existing := cnames[0]
				if shouldReplaceExisting(ri, existing, logger) {
					desiredByNameKind.Get(ri.Record.Name).Get(domain.RecordCNAME).Delete(existing.Record.Value)
					desiredByNameKind.Get(ri.Record.Name).Get(domain.RecordCNAME).Set(ri.Record.Value, ri)
				}
			} else {
				// No conflict - just add it
				desiredByNameKind.Get(ri.Record.Name).Get(domain.RecordCNAME).Set(ri.Record.Value, ri)
			}
		}
	}

	// 2. Resolve CNAME vs A/AAAA conflict per name
	desiredByNameKindDeduplicated := newNestedRecordMap()
	for _, name := range desiredByNameKind.GetAllNames() {
		aRecs, hasA := desiredByNameKind.PeekNameKindRecords(name, domain.RecordA)
		aaaaRecs, hasAAAA := desiredByNameKind.PeekNameKindRecords(name, domain.RecordAAAA)
		cnameRecs, hasCNAME := desiredByNameKind.PeekNameKindRecords(name, domain.RecordCNAME)

		switch {
		case hasCNAME && !(hasA || hasAAAA):
			// Keep the single CNAME
			ri := cnameRecs[0]
			desiredByNameKindDeduplicated.Get(name).Get(domain.RecordCNAME).Set(ri.Record.Value, ri)

		case (hasA || hasAAAA) && !hasCNAME:
			for _, ri := range aRecs {
				desiredByNameKindDeduplicated.Get(name).Get(domain.RecordA).Set(ri.Record.Value, ri)
			}
			for _, ri := range aaaaRecs {
				desiredByNameKindDeduplicated.Get(name).Get(domain.RecordAAAA).Set(ri.Record.Value, ri)
			}

		case hasCNAME && (hasA || hasAAAA):
			cnameRecord := cnameRecs[0]
			allAddr, _ := addrRecords(desiredByNameKind, name)

			if shouldReplaceAllExisting(cnameRecord, allAddr, logger) {
				desiredByNameKindDeduplicated.Get(name).Get(domain.RecordCNAME).Set(cnameRecord.Record.Value, cnameRecord)
			} else {
				for _, ri := range aRecs {
					desiredByNameKindDeduplicated.Get(name).Get(domain.RecordA).Set(ri.Record.Value, ri)
				}
				for _, ri := range aaaaRecs {
					desiredByNameKindDeduplicated.Get(name).Get(domain.RecordAAAA).Set(ri.Record.Value, ri)
				}
			}

		default:
			logger.Warn().Str("name", name).Msg("Found a record name with no supported record kind. Skipping.")
		}
	}

	return desiredByNameKindDeduplicated.GetAllValues()
}

func ReconcileAndValidate(desired, actual []*domain.RecordIntent, cfg *config.AppConfig, logger zerolog.Logger) ([]*domain.RecordIntent, []*domain.RecordIntent) {
	toAddMap := map[string]*domain.RecordIntent{}
	toRemoveMap := map[string]*domain.RecordIntent{}

	actualByNameKind := newNestedRecordMap()
	desiredSet := make(map[string]struct{}, len(desired))
	for _, ri := range desired {
		desiredSet[ri.Key()] = struct{}{}
	}

	// Step 1: Remove stale records and build lookup structure
	for _, ri := range actual {
		if _, exists := desiredSet[ri.Key()]; !exists {
			if ri.Hostname == cfg.Hostname {
				logger.Info().Msgf("Removing stale record: %s (owned by %s/%s)", ri.Record.Render(), ri.Hostname, ri.ContainerName)
				toRemoveMap[ri.Key()] = ri
			} else {
				logger.Debug().Msgf("Skipping removal of record %s not owned by this host (%s != %s)", ri.Record.Render(), ri.Hostname, cfg.Hostname)
			}
		} else {
			actualByNameKind.Get(ri.Record.Name).Get(ri.Record.Kind).Set(ri.Record.Value, ri)
		}
	}

	// Step 2: Reconcile each desired record
	for _, d := range desired {
		evictions := map[string]*domain.RecordIntent{}

		switch {
		case d.Record.IsA():
			// A vs CNAME
			if cnames, ok := actualByNameKind.PeekNameKindRecords(d.Record.Name, domain.RecordCNAME); ok {
				existing := cnames[0]
				if d.Force || d.Created.Before(existing.Created) {
					for _, r := range cnames {
						evictions[r.Key()] = r
					}
					logger.Warn().Strs("actual", renderAll(cnames)).Str("desired", d.Render()).Bool("force_eviction", d.Force).Bool("age_eviction", d.Created.Before(existing.Created)).Msg("A vs CNAME - evicting CNAME")
				} else {
					continue
				}
			} else if r, ok := actualByNameKind.PeekNameKindRecord(d.Record.Name, domain.RecordA, d.Record.Value); ok {
				// Same A exists - replace only if d wins
				if r.Equal(*d) {
					continue
				} else if d.Force || d.Created.Before(r.Created) {
					logger.Warn().Str("actual_record_intent", r.Render()).Str("desired", d.Render()).Bool("force_eviction", d.Force).Bool("age_eviction", d.Created.Before(r.Created)).Msg("A vs A - evicting A")
					evictions[r.Key()] = r
				} else {
					continue
				}
			}

		case d.Record.IsAAAA():
			if cnames, ok := actualByNameKind.PeekNameKindRecords(d.Record.Name, domain.RecordCNAME); ok {
				existing := cnames[0]
				if d.Force || d.Created.Before(existing.Created) {
					for _, r := range cnames {
						evictions[r.Key()] = r
					}
					logger.Warn().Strs("actual", renderAll(cnames)).Str("desired", d.Render()).Bool("force_eviction", d.Force).Bool("age_eviction", d.Created.Before(existing.Created)).Msg("AAAA vs CNAME - evicting CNAME")
				} else {
					continue
				}
			} else if r, ok := actualByNameKind.PeekNameKindRecord(d.Record.Name, domain.RecordAAAA, d.Record.Value); ok {
				// Same AAAA exists — replace only if d wins
				if r.Equal(*d) {
					continue
				} else if d.Force || d.Created.Before(r.Created) {
					logger.Warn().Str("actual_record_intent", r.Render()).Str("desired", d.Render()).Bool("force_eviction", d.Force).Bool("age_eviction", d.Created.Before(r.Created)).Msg("AAAA vs AAAA - evicting AAAA")
					evictions[r.Key()] = r
				} else {
					continue
				}
			}

		case d.Record.IsCNAME():
			// CNAME vs A/AAAA
			if allAddr, hasAddr := addrRecords(actualByNameKind, d.Record.Name); hasAddr {
				olderThanAll := true
				for _, r := range allAddr {
					if !d.Created.Before(r.Created) {
						olderThanAll = false
						break
					}
				}

				if d.Force || olderThanAll {
					for _, r := range allAddr {
						evictions[r.Key()] = r
					}
					logger.Warn().Strs("actual", renderAll(allAddr)).Str("desired", d.Render()).Bool("force_eviction", d.Force).Bool("age_eviction", olderThanAll).Msg("CNAME vs A/AAAA — evicting address records")
				} else {
					continue
				}
			} else if cnames, ok := actualByNameKind.PeekNameKindRecords(d.Record.Name, domain.RecordCNAME); ok {
				existing := cnames[0]
				if existing.Equal(*d) {
					continue
				}
				if d.Force || d.Created.Before(existing.Created) {
					for _, r := range cnames {
						evictions[r.Key()] = r
					}
					logger.Warn().Strs("actual", renderAll(cnames)).Str("desired", d.Render()).Bool("force_eviction", d.Force).Bool("age_eviction", d.Created.Before(existing.Created)).Msg("Local vs remote - evicting remote")
				} else {
					continue
				}
			}

		default:
			// Don't skip - just add with no evictions - no need for an else statement, this will just work
		}

		// Step 3: Simulate state for validation
		keysToRemove := map[string]struct{}{}
		for k := range toRemoveMap {
			keysToRemove[k] = struct{}{}
		}
		for key := range evictions {
			keysToRemove[key] = struct{}{}
		}

		var simulated []*domain.RecordIntent
		for _, r := range actual {
			if _, skip := keysToRemove[r.Key()]; !skip {
				simulated = append(simulated, r)
			}
		}

		if err := ValidateRecord(d, simulated, logger); err == nil {
			logger.Info().Str("record", d.Render()).Msg("Adding new record")
			toAddMap[d.Record.Key()] = d
			for k, v := range evictions {
				toRemoveMap[k] = v
			}
		} else {
			logger.Warn().Err(err).Str("record", d.Record.Render()).Msg("Skipping invalid record")
		}
	}

	// Step 5: Convert maps to slices
	toAdd := make([]*domain.RecordIntent, 0, len(toAddMap))
	toRemove := make([]*domain.RecordIntent, 0, len(toRemoveMap))
	for _, r := range toAddMap {
		toAdd = append(toAdd, r)
	}
	for _, r := range toRemoveMap {
		toRemove = append(toRemove, r)
	}
	return toAdd, toRemove
}

func addrRecords(m *nestedRecordMap, name string) ([]*domain.RecordIntent, bool) {
	a, hasA := m.PeekNameKindRecords(name, domain.RecordA)
	aaaa, hasAAAA := m.PeekNameKindRecords(name, domain.RecordAAAA)
	if !hasA && !hasAAAA {
		return nil, false
	}
	out := make([]*domain.RecordIntent, 0, len(a)+len(aaaa))
	out = append(out, a...)
	out = append(out, aaaa...)
	return out, true
}

func renderAll(rs []*domain.RecordIntent) []string {
	out := make([]string, len(rs))
	for i, r := range rs {
		out[i] = r.Render()
	}
	return out
}
