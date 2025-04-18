package core

import (
	"context"
	"fmt"
	"time"

	"github.com/StevenC4/docker-coredns-sync/internal/config"
	"github.com/StevenC4/docker-coredns-sync/internal/registry"
	"github.com/rs/zerolog"
)

// SyncEngine coordinates event ingestion, state updates, and registry reconciliation.
type SyncEngine struct {
	logger       zerolog.Logger
	cfg          *config.AppConfig
	watcher      DockerWatcher
	stateTracker *StateTracker
	registry     registry.Registry
}

func NewSyncEngine(logger zerolog.Logger, cfg *config.AppConfig, watcher DockerWatcher, reg registry.Registry) *SyncEngine {
	return &SyncEngine{
		logger:       logger,
		cfg:          cfg,
		watcher:      watcher,
		stateTracker: NewStateTracker(),
		registry:     reg,
	}
}

func (se *SyncEngine) prepopulateState(ctx context.Context) error {
	// You likely need a Docker client call to list current containers.
	containers, err := se.watcher.ListRunningContainers(ctx)
	if err != nil {
		return err
	}
	for _, container := range containers {
		// Convert container details to a ContainerEvent.
		evt := ContainerEvent{
			ID:      container.ID,
			Name:    container.Name,
			Status:  container.Status,
			Created: container.Created,
			Labels:  container.Labels,
		}
		// Build record intents for this container.
		intents, err := GetContainerRecordIntents(evt, se.cfg, se.logger)
		if err != nil {
			se.logger.Error().Err(err).Msg("Error building record intents during prepopulation")
			continue
		}
		if len(intents) > 0 {
			se.stateTracker.Upsert(evt.ID, evt.Name, evt.Created, intents, "running")
			se.logger.Info().Msgf("Prepopulated state for container %s", container.Name)
		}
	}
	return nil
}

func (se *SyncEngine) handleEvent(evt ContainerEvent) {
	if evt.ID == "" {
		return
	}
	if evt.Status == "start" {
		intents, err := GetContainerRecordIntents(evt, se.cfg, se.logger)
		if err != nil {
			se.logger.Error().Err(err).Msg("Error building record intents")
			return
		}
		// If intents are returned, update the state tracker.
		if len(intents) > 0 {
			se.stateTracker.Upsert(evt.ID, evt.Name, evt.Created, intents, "running")
			se.logger.Info().Msgf("Upserted state for container %s", evt.ID)
		}
	} else {
		if removed := se.stateTracker.MarkRemoved(evt.ID); removed {
			se.logger.Info().Msgf("Marked container %s as removed", evt.ID)
		}
	}
}

func (se *SyncEngine) Run(ctx context.Context) error {
	se.logger.Info().Msg("Starting SyncEngine")

	// Step 1: Subscribe to Docker events (assume this is done elsewhere)
	eventCh, err := se.watcher.Subscribe(ctx)
	if err != nil {
		return fmt.Errorf("Failed to subscribe to Docker events: %w", err)
	}

	// Step 2: Prepopulate state by listing running containers.
	se.logger.Info().Msg("Prepopulating the state with currently running containers")
	if err := se.prepopulateState(ctx); err != nil {
		se.logger.Error().Err(err).Msg("Error during state prepopulation")
	}

	// Step 3. Drain any events that arrived during prepopulation.
	// Use a non-blocking drain loop with a short timeout.
	se.logger.Info().Msg("Draining events that arrived during container prepopulation")
	drainTimeout := time.After(500 * time.Millisecond)
	drained := false
	for !drained {
		select {
		case evt, ok := <-eventCh:
			if !ok {
				se.logger.Info().Msg("Queue drained")
				drained = true
				break
			}
			se.handleEvent(evt)
		case <-drainTimeout:
			se.logger.Info().Msg("Drain timeout reached")
			drained = true
		}
	}

	// Step 4: Launch a goroutine to process incoming events and update the state tracker.
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

	// Step 5: Launch the main reconciliation loop.
	se.logger.Info().Msg("Launching reconciliation loop")
	ticker := time.NewTicker(time.Duration(se.cfg.PollInterval) * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			se.logger.Debug().Msg("Reconciliation loop tick")
			err := se.registry.LockTransaction(ctx, []string{"__global__"}, func() error {
				actual, err := se.registry.List(ctx)
				if err != nil {
					return fmt.Errorf("Error listing registry records: %w", err)
				}
				desired := se.stateTracker.GetAllDesiredRecordIntents()
				// Filter out any internally inconsistent intents:
				desiredReconciled := FilterRecordIntents(desired, se.logger)
				toAdd, toRemove := ReconcileAndValidate(desiredReconciled, actual, se.cfg, se.logger)
				for _, rec := range toRemove {
					if err := se.registry.Remove(ctx, rec); err != nil {
						se.logger.Error().Err(err).Msg("Error removing record")
					}
				}
				for _, rec := range toAdd {
					if err := se.registry.Register(ctx, rec); err != nil {
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
			se.watcher.Stop()
			err := se.registry.Close()
			if err != nil {
				se.logger.Error().Err(err).Msg("Error closing registry")
			}
			return ctx.Err()
		}
	}
}
