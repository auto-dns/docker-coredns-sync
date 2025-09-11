package core

import (
	"strings"

	"github.com/auto-dns/docker-coredns-sync/internal/config"
	"github.com/auto-dns/docker-coredns-sync/internal/domain"
	"github.com/rs/zerolog"
)

// GetContainerRecordIntents parses the container event's labels and returns record intents.
func GetContainerRecordIntents(event domain.ContainerEvent, cfg *config.AppConfig, logger zerolog.Logger) ([]*domain.RecordIntent, error) {
	var intents []*domain.RecordIntent

	prefix := cfg.DockerLabelPrefix
	parsedLabels := ParseLabels(prefix, event.Container.Labels)

	if !parsedLabels.Enabled {
		logger.Debug().Msg("Record generation not enabled for container")
		return intents, nil
	}

	for _, labeledRecord := range parsedLabels.Records {
		// Handle empty name
		// -- Skip
		if strings.TrimSpace(labeledRecord.Name) == "" {
			logger.Warn().Str("kind", string(labeledRecord.Kind)).Msg("skipping record with no name")
			continue
		}

		// Handle empty value
		// -- For A and AAAA, use config value for default
		value := strings.TrimSpace(labeledRecord.Value)
		if value == "" {
			nameLabel := labeledRecord.GetNameLabel()
			valueLabel := labeledRecord.GetValueLabel()
			switch labeledRecord.Kind {
			case domain.RecordA:
				value = cfg.HostIPv4
				if value == "" {
					logger.Warn().Str("name", labeledRecord.Name).Msgf("%s label found with no matching %s. No default value configured. Skipping.", nameLabel, valueLabel)
					continue
				} else {
					logger.Debug().Str("name", labeledRecord.Name).Msgf("%s label found with no matching %s. Using host IP %s", nameLabel, valueLabel, value)
				}
			case domain.RecordAAAA:
				value = cfg.HostIPv6
				if value == "" {
					logger.Warn().Str("name", labeledRecord.Name).Msgf("%s label found with no matching %s. No default value configured. Skipping.", nameLabel, valueLabel)
					continue
				} else {
					logger.Debug().Str("name", labeledRecord.Name).Msgf("%s label found with no matching %s. Using host IP %s", nameLabel, valueLabel, value)
				}
			case domain.RecordCNAME:
				logger.Warn().Str("name", labeledRecord.Name).Msgf("%s label found with no matching %s. Skipping.", nameLabel, valueLabel)
				continue
			default:
				logger.Warn().Str("record_type", string(labeledRecord.Kind)).Msg("unsupported record type")
			}
		}

		rec, err := domain.NewFromKind(labeledRecord.Kind, labeledRecord.Name, value)
		if err != nil {
			logger.Warn().Err(err).Str("kind", string(labeledRecord.Kind)).Str("name", labeledRecord.Name).Str("value", value).Msg("invalid record")
			continue
		}

		force := parsedLabels.ContainerForce
		if labeledRecord.Force != nil {
			force = *labeledRecord.Force
		}

		intent := &domain.RecordIntent{
			ContainerId:   event.Container.Id,
			ContainerName: event.Container.Name,
			Created:       event.Container.Created,
			Hostname:      cfg.Hostname,
			Force:         force,
			Record:        rec,
		}
		intents = append(intents, intent)
	}

	return intents, nil
}
