package core

import (
	"fmt"
	"strings"

	"github.com/auto-dns/docker-coredns-sync/internal/config"
	"github.com/auto-dns/docker-coredns-sync/internal/domain"
	"github.com/rs/zerolog"
)

// getForce determines the "force" flag value based on label values.
func getForce(labels map[string]string, containerForceLabel, recordForceLabel string) bool {
	containerForce := strings.ToLower(labels[containerForceLabel]) == "true"
	_, recordForceLabelExists := labels[recordForceLabel]
	recordForce := strings.ToLower(labels[recordForceLabel]) == "true"
	if recordForceLabelExists {
		return recordForce
	}
	return containerForce
}

// GetContainerRecordIntents parses the container event's labels and returns record intents.
func GetContainerRecordIntents(event domain.ContainerEvent, cfg *config.AppConfig, logger zerolog.Logger) ([]*domain.RecordIntent, error) {
	var intents []*domain.RecordIntent

	labels := event.Container.Labels
	prefix := cfg.DockerLabelPrefix

	// If the feature is disabled in labels, return empty.
	if strings.ToLower(labels[prefix+".enabled"]) != "true" {
		logger.Debug().Msg("Record generation disabled via label")
		return intents, nil
	}

	// We will collect base label pairs (without aliases)
	baseLabelPairs := make(map[string]map[string]string) // key: record type ("A" or "CNAME")
	// And aliased label pairs.
	aliasedLabelPairs := make(map[string]map[string]map[string]string) // key: record type, then alias

	// Process each label.
	for label, value := range labels {
		if !strings.HasPrefix(label, prefix) {
			continue
		}

		parts := strings.Split(label, ".")
		// We expect at least three parts: e.g., "coredns.A.name"
		if len(parts) < 3 {
			logger.Debug().Msgf("Skipping malformed label: %s", label)
			continue
		}

		recordType := strings.ToUpper(parts[1])
		switch {
		case recordType == "A" && !cfg.RecordTypes.A.Enabled,
			recordType == "AAAA" && !cfg.RecordTypes.AAAA.Enabled,
			recordType == "CNAME" && !cfg.RecordTypes.CNAME.Enabled:
			logger.Warn().Msgf("Skipping disabled label '%s' for disabled record type '%s'", label, recordType)
			continue
		case recordType == "enabled",
			recordType == "force":
			// Known record types
		default:
			logger.Warn().Msgf("Skipping unsupported record type '%s' for label '%s'", recordType, label)
			continue
		}

		// Two formats: base format (three parts) or aliased (at least four parts).
		if len(parts) == 3 && (parts[2] == "name" || parts[2] == "value") {
			key := parts[2]
			if _, exists := baseLabelPairs[recordType]; !exists {
				baseLabelPairs[recordType] = make(map[string]string)
			}
			baseLabelPairs[recordType][key] = value
		} else if len(parts) >= 4 && (parts[3] == "name" || parts[3] == "value") {
			alias := parts[2]
			key := parts[3]
			if _, exists := aliasedLabelPairs[recordType]; !exists {
				aliasedLabelPairs[recordType] = make(map[string]map[string]string)
			}
			if _, exists := aliasedLabelPairs[recordType][alias]; !exists {
				aliasedLabelPairs[recordType][alias] = make(map[string]string)
			}
			aliasedLabelPairs[recordType][alias][key] = value
		}
	}

	recordIntents := []*domain.RecordIntent{}
	containerID := event.Container.Id
	containerName := event.Container.Name
	containerCreated := event.Container.Created
	hostname := cfg.Hostname
	containerForceLabel := prefix + ".force"

	// Process base label pairs for A records.
	for recordType, kv := range baseLabelPairs {
		name, nameOk := kv["name"]
		value, valueOk := kv["value"]
		if !nameOk {
			logger.Warn().Msgf("Skipping - %s.%s.value label found with no matching name", prefix, recordType)
			continue
		}
		if !valueOk {
			if recordType == "A" {
				value = cfg.HostIP
				logger.Warn().Msgf("%s.A.name label found with no matching %s.A.value. Using host IP %s", prefix, prefix, value)
			} else {
				logger.Warn().Msgf("Skipping - %s.%s.name label found with no matching value", prefix, recordType)
				continue
			}
		}

		rec, err := domain.NewFromString(recordType, name, value)
		if err != nil {
			logger.Error().Str("record_type", recordType).Msgf("parsing record")
			continue
		}
		force := getForce(labels, containerForceLabel, fmt.Sprintf("%s.%s.force", prefix, strings.ToLower(recordType)))

		intent := &domain.RecordIntent{
			ContainerID:   containerID,
			ContainerName: containerName,
			Created:       containerCreated,
			Force:         force,
			Hostname:      hostname,
			Record:        rec,
		}
		recordIntents = append(recordIntents, intent)
	}

	// Process aliased label pairs for all record types
	for recordType, aliases := range aliasedLabelPairs {
		for alias, kv := range aliases {
			name, nameOk := kv["name"]
			value, valueOk := kv["value"]
			if !nameOk {
				logger.Warn().Msgf("Skipping - %s.%s.%s.value label found with no matching name", prefix, recordType, alias)
				continue
			}
			if !valueOk {
				if recordType == "A" {
					value = cfg.HostIP
					logger.Warn().Msgf("%s.A.%s.name label found with no matching %s.A.%s.value. Using host IP %s", prefix, alias, prefix, alias, value)
				} else {
					logger.Warn().Msgf("Skipping - %s.%s.%s.name label found with no matching value", prefix, recordType, alias)
					continue
				}
			}

			rec, err := domain.NewFromString(recordType, name, value)
			if err != nil {
				logger.Error().Str("alias", alias).Str("record_type", recordType).Msg("parsing record")
				continue
			}
			force := getForce(labels, containerForceLabel, fmt.Sprintf("%s.%s.%s.force", prefix, strings.ToLower(recordType), alias))

			intent := &domain.RecordIntent{
				ContainerID:   containerID,
				ContainerName: containerName,
				Created:       containerCreated,
				Force:         force,
				Hostname:      hostname,
				Record:        rec,
			}
			recordIntents = append(recordIntents, intent)
		}
	}

	return recordIntents, nil
}
