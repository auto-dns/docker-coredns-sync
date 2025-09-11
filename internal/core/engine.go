package core

import (
	"context"
	"fmt"
	"time"

	"github.com/auto-dns/docker-coredns-sync/internal/config"
	"github.com/auto-dns/docker-coredns-sync/internal/domain"
	"github.com/rs/zerolog"
)

// SyncEngine coordinates event ingestion, state updates, and registry reconciliation.
type SyncEngine struct {
	logger zerolog.Logger
	cfg    *config.AppConfig
	gen    generator
	state  state
	reg    upstreamRegistry
}

func NewSyncEngine(logger zerolog.Logger, cfg *config.AppConfig, gen generator, reg upstreamRegistry, state state) *SyncEngine {
	return &SyncEngine{
		logger: logger,
		cfg:    cfg,
		gen:    gen,
		reg:    reg,
		state:  state,
	}
}

func (se *SyncEngine) handleEvent(evt domain.ContainerEvent) {
	switch {
	case evt.Container.Id == "":
		se.logger.Warn().Str("event_payload", fmt.Sprintf("%+v", evt)).Msg("handled container event with no container id")
	case !evt.EventType.IsValid():
		se.logger.Warn().Str("container_id", evt.Container.Id).Str("event_type", string(evt.EventType)).Msg("handled unsupported event type")
	case evt.EventType == domain.EventTypeInitialContainerDetection, evt.EventType == domain.EventTypeContainerStarted:
		intents, err := GetContainerRecordIntents(evt, se.cfg, se.logger)
		if err != nil {
			se.logger.Error().Err(err).Msg("Error building record intents")
			return
		}
		// If intents are returned, update the state tracker.
		if len(intents) > 0 {
			se.state.Upsert(evt.Container.Id, evt.Container.Name, evt.Container.Created, intents, "running")
			se.logger.Info().Msgf("Upserted state for container %s", evt.Container.Id)
		}
	case evt.EventType == domain.EventTypeContainerStopped, evt.EventType == domain.EventTypeContainerDied:
		if removed := se.state.MarkRemoved(evt.Container.Id); removed {
			se.logger.Info().Msgf("Marked container %s as removed", evt.Container.Id)
		}
	}
}

func (se *SyncEngine) Run(ctx context.Context) error {
	se.logger.Info().Msg("Starting SyncEngine")

	// Step 1: Subscribe to Docker events
	eventCh, err := se.gen.Subscribe(ctx)
	if err != nil {
		return fmt.Errorf("Failed to subscribe to Docker events: %w", err)
	}

	// Step 2: Launch a goroutine to process incoming events and update the state tracker.
	se.logger.Info().Msg("Launching event processing goroutine")
	go func() {
		for {
			select {
			case evt, ok := <-eventCh:
				if !ok {
					se.logger.Info().Msg("Event channel closed")
					return
				}
				se.handleEvent(evt)
			case <-ctx.Done():
				se.logger.Info().Msg("Stopping event processing")
				return
			}
		}
	}()

	// Step 3: Launch the main reconciliation loop.
	se.logger.Info().Msg("Launching reconciliation loop")
	ticker := time.NewTicker(time.Duration(se.cfg.PollInterval) * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			se.logger.Debug().Msg("Reconciliation loop tick")
			err := se.reg.LockTransaction(ctx, []string{"__global__"}, func() error {
				actual, err := se.reg.List(ctx)
				if err != nil {
					return fmt.Errorf("Error listing registry records: %w", err)
				}
				desired := se.state.GetAllDesiredRecordIntents()
				// Filter out any internally inconsistent intents:
				desiredReconciled := FilterRecordIntents(desired, se.logger)
				toAdd, toRemove := ReconcileAndValidate(desiredReconciled, actual, se.cfg, se.logger)
				for _, rec := range toRemove {
					if err := se.reg.Remove(ctx, rec); err != nil {
						se.logger.Error().Err(err).Msg("Error removing record")
					}
				}
				for _, rec := range toAdd {
					if err := se.reg.Register(ctx, rec); err != nil {
						se.logger.Error().Err(err).Msg("Error registering record")
					}
				}
				return nil
			})
			if err != nil {
				se.logger.Error().Err(err).Msg("Sync error")
			}
		case <-ctx.Done():
			se.logger.Info().Msg("SyncEngine shutting down")
			err := se.reg.Close()
			if err != nil {
				se.logger.Error().Err(err).Msg("Error closing registry")
			}
			return ctx.Err()
		}
	}
}
