package event

import (
	"context"
	"time"

	"github.com/auto-dns/docker-coredns-sync/internal/domain"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/api/types/filters"
	"github.com/rs/zerolog"
)

type DockerGenerator struct {
	logger zerolog.Logger
	cli    dockerClient
}

func NewDockerGenerator(cli dockerClient, logger zerolog.Logger) *DockerGenerator {
	return &DockerGenerator{
		logger: logger,
		cli:    cli,
	}
}

func (dw *DockerGenerator) Subscribe(ctx context.Context) (<-chan domain.ContainerEvent, error) {
	const bufferSize = 100 // TODO: config this
	out := make(chan domain.ContainerEvent, bufferSize)

	go func() {
		defer close(out)
		defer func() { _ = dw.cli.Close() }()

		since := time.Now()

		// Process initial list of containers
		opts := container.ListOptions{All: false}
		containers, err := dw.cli.ContainerList(ctx, opts)
		if err != nil {
			dw.logger.Error().Err(err).Msg("getting list of containers")
			return
		}
		for _, c := range containers {
			select {
			case out <- fromContainerToDomainContainerEvent(c):
			case <-ctx.Done():
				dw.logger.Info().Msg("Docker event generator cancelled during initial emit")
				return
			}
		}

		// Set up channel to get docker container events
		// Create a filter to get container events
		filterArgs := filters.NewArgs()
		filterArgs.Add("type", "container")
		filterArgs.Add("event", "start")
		filterArgs.Add("event", "stop")
		filterArgs.Add("event", "die")

		options := events.ListOptions{
			Filters: filterArgs,
			Since:   since.Format(time.RFC3339Nano),
		}
		eventCh, errCh := dw.cli.Events(ctx, options)

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

				event, convErr := fromMsgToDomainContainerEvent(msg)
				if convErr != nil {
					if _, ok := convErr.(*UnsupportedEventTypeError); ok {
						dw.logger.Debug().Err(convErr).Msg("Error converting docker event message to container event")
					} else {
						dw.logger.Error().Err(convErr).Msg("converting docker event message to container event")
					}
					continue
				}

				dw.logger.Debug().Msgf("Received Docker event: %+v", event)
				select {
				case out <- event:
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	return out, nil
}

func fromContainerToDomainContainerEvent(c container.Summary) domain.ContainerEvent {
	return domain.ContainerEvent{
		Container: domain.Container{
			Id:      c.ID,
			Name:    c.Names[0],
			Created: time.Unix(c.Created, 0),
			Labels:  c.Labels,
		},
		EventType: domain.EventTypeInitialContainerDetection,
	}
}

func fromMsgToDomainContainerEvent(msg events.Message) (domain.ContainerEvent, error) {
	event := domain.ContainerEvent{
		Container: domain.Container{
			Id:      msg.ID,
			Name:    msg.Actor.Attributes["name"],
			Created: time.Unix(0, msg.TimeNano),
			Labels:  msg.Actor.Attributes,
		},
		EventType: domain.EventType(msg.Status),
	}

	if !event.EventType.IsValid() {
		return domain.ContainerEvent{}, NewUnsupportedEventTypeError(event.EventType)
	}

	return event, nil
}
