package core

import (
	"context"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/api/types/filters"
	"github.com/rs/zerolog"
)

type dockerClient interface {
	Events(ctx context.Context, options events.ListOptions) (<-chan events.Message, <-chan error)
	ContainerList(ctx context.Context, options container.ListOptions) ([]container.Summary, error)
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

func (dw *DockerWatcherImpl) Subscribe(ctx context.Context) (<-chan ContainerEvent, error) {
	const bufferSize = 100 // TODO: config this
	out := make(chan ContainerEvent, bufferSize)

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

func (dw *DockerWatcherImpl) ListRunningContainers(ctx context.Context) ([]ContainerEvent, error) {
	opts := container.ListOptions{All: false}
	containers, err := dw.cli.ContainerList(ctx, opts)
	if err != nil {
		return nil, err
	}
	var events []ContainerEvent
	for _, c := range containers {
		// Here, we assume at least one name is present. You might need
		// to adjust this logic if there can be more than one.
		evt := ContainerEvent{
			ID:      c.ID,
			Name:    c.Names[0],
			Status:  "running",
			Created: time.Unix(c.Created, 0),
			Labels:  c.Labels,
		}
		events = append(events, evt)
	}
	return events, nil
}

func (dw *DockerWatcherImpl) Stop() {
	dw.logger.Info().Msg("Stopping Docker watcher")
	_ = dw.cli.Close()
}
