package core

import (
	"context"
	"time"

	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/api/types/filters"
	"github.com/rs/zerolog"
)

type dockerClient interface {
	Events(ctx context.Context, options events.ListOptions) (<-chan events.Message, <-chan error)
	Close() error
}

type DockerWatcherImpl struct {
	logger zerolog.Logger
	cli    dockerClient
}

func NewDockerWatcherImpl(cli dockerClient, logger zerolog.Logger) DockerWatcher {
	return &DockerWatcherImpl{
		logger: logger,
		cli:    cli,
	}
}

// Subscribe connects to the Docker event stream and converts events into ContainerEvent objects.
func (dw *DockerWatcherImpl) Subscribe(ctx context.Context) (<-chan ContainerEvent, error) {
	out := make(chan ContainerEvent)

	// Create a filter: we listen for container events.
	filterArgs := filters.NewArgs()
	filterArgs.Add("type", "container")
	filterArgs.Add("event", "start")
	filterArgs.Add("event", "stop")
	filterArgs.Add("event", "die")

	options := events.ListOptions{
		Filters: filterArgs,
	}

	eventCh, errCh := dw.cli.Events(ctx, options)

	go func() {
		defer close(out)
		for {
			select {
			case <-ctx.Done():
				dw.logger.Info().Msg("Docker watcher cancelled by context")
				return
			case err, ok := <-errCh:
				if ok && err != nil {
					dw.logger.Error().Err(err).Msg("Error from Docker events stream")
				}
			case msg, ok := <-eventCh:
				if !ok {
					dw.logger.Info().Msg("Docker events channel closed")
					return
				}
				// Filter for statuses you want to handle (if needed, additional filtering here)
				if msg.Status != "start" && msg.Status != "stop" && msg.Status != "die" {
					continue
				}
				// Construct ContainerEvent.
				evt := ContainerEvent{
					ID:      msg.ID,
					Status:  msg.Status,
					Created: time.Unix(0, msg.TimeNano),
					Name:    msg.Actor.Attributes["name"],
					Labels:  msg.Actor.Attributes,
				}
				dw.logger.Debug().Msgf("Received Docker event: %+v", evt)
				out <- evt
			}
		}
	}()

	return out, nil
}

// Stop closes the Docker client. (You can rely on context cancellation for stopping events.)
func (dw *DockerWatcherImpl) Stop() {
	dw.logger.Info().Msg("Stopping Docker watcher")
	_ = dw.cli.Close()
}
