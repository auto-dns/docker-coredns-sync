package core

import (
	"strings"

	"github.com/StevenC4/docker-coredns-sync/internal/config"
	"github.com/StevenC4/docker-coredns-sync/internal/dns"
	"github.com/StevenC4/docker-coredns-sync/internal/intent"
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
func GetContainerRecordIntents(event ContainerEvent, settings *config.AppConfig, logger zerolog.Logger) ([]*intent.RecordIntent, error) {
	var intents []*intent.RecordIntent

	labels := event.Labels
	prefix := settings.DockerLabelPrefix

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

		recordType := parts[1]
		// Check if recordType is allowed.
		allowed := false
		for _, rt := range settings.AllowedRecordTypes {
			if recordType == rt {
				allowed = true
				break
			}
		}
		// Skip unknown types except non-record labels (like "enabled" or "force").
		if !allowed && recordType != "enabled" && recordType != "force" {
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

	recordIntents := []*intent.RecordIntent{}
	containerID := event.ID
	containerName := event.Name
	containerCreated := event.Created
	hostname := settings.Hostname
	containerForceLabel := prefix + ".force"

	// Process base label pairs for A records.
	if aLabels, exists := baseLabelPairs["A"]; exists {
		if name, ok := aLabels["name"]; ok {
			var value string
			if v, ok := aLabels["value"]; ok {
				value = v
			} else {
				value = settings.HostIP
				logger.Warn().Msgf("%s.A.name label found with no matching %s.A.value. Using host IP %s", prefix, prefix, value)
			}
			force := getForce(labels, containerForceLabel, prefix+".A.force")
			aRec, err := dns.NewARecord(name, value)
			if err != nil {
				logger.Warn().Msgf("Invalid ARecord %s: %v", name, err)
			} else {
				intent := &intent.RecordIntent{
					ContainerID:   containerID,
					ContainerName: containerName,
					Created:       containerCreated,
					Force:         force,
					Hostname:      hostname,
					Record:        aRec,
				}
				recordIntents = append(recordIntents, intent)
			}
		} else if _, exists := aLabels["value"]; exists {
			logger.Error().Msgf("%s.A.value label found with no matching %s.A.name", prefix, prefix)
		}
	}

	// Process base label pairs for CNAME records.
	if cnameLabels, exists := baseLabelPairs["CNAME"]; exists {
		if name, nameOk := cnameLabels["name"]; nameOk {
			if value, valueOk := cnameLabels["value"]; valueOk {
				force := getForce(labels, containerForceLabel, prefix+".CNAME.force")
				cnameRec, err := dns.NewCNAMERecord(name, value)
				if err != nil {
					logger.Warn().Msgf("Invalid CNAMERecord %s: %v", name, err)
				} else {
					intent := &intent.RecordIntent{
						ContainerID:   containerID,
						ContainerName: containerName,
						Created:       containerCreated,
						Force:         force,
						Hostname:      hostname,
						Record:        cnameRec,
					}
					recordIntents = append(recordIntents, intent)
				}
			} else {
				logger.Error().Msgf("%s.CNAME.name label found with no matching %s.CNAME.value", prefix, prefix)
			}
		} else if _, exists := cnameLabels["value"]; exists {
			logger.Error().Msgf("%s.CNAME.value label found with no matching %s.CNAME.name", prefix, prefix)
		}
	}

	// Process aliased label pairs similarly...
	// [Omitted for brevity. Follow similar pattern: iterate over each alias,
	// validate presence of both "name" and "value", construct the record,
	// determine force, and append to recordIntents.]

	return recordIntents, nil
}
