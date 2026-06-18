package event

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"github.com/auto-dns/docker-coredns-sync/internal/domain"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/api/types/filters"
	"github.com/rs/zerolog"
)

const (
	defaultEventBufferSize     = 100
	defaultReconnectInitial    = 1 * time.Second
	defaultReconnectMaxBackoff = 30 * time.Second
)

type DockerGenerator struct {
	logger             zerolog.Logger
	cli                dockerClient
	onConnectionChange func(connected bool)
	bufferSize         int
	reconnectInitial   time.Duration
	reconnectMax       time.Duration
}

func NewDockerGenerator(cli dockerClient, logger zerolog.Logger) *DockerGenerator {
	return &DockerGenerator{
		logger:           logger,
		cli:              cli,
		bufferSize:       defaultEventBufferSize,
		reconnectInitial: defaultReconnectInitial,
		reconnectMax:     defaultReconnectMaxBackoff,
	}
}

// SetConnectionObserver registers an optional callback invoked when the Docker
// event stream connects (true) or disconnects (false). Safe to leave unset.
func (dw *DockerGenerator) SetConnectionObserver(fn func(connected bool)) {
	dw.onConnectionChange = fn
}

// SetEventBufferSize overrides the size of the buffered output channel.
func (dw *DockerGenerator) SetEventBufferSize(n int) {
	if n > 0 {
		dw.bufferSize = n
	}
}

// SetReconnectBackoff overrides the reconnect backoff bounds.
func (dw *DockerGenerator) SetReconnectBackoff(initial, max time.Duration) {
	if initial > 0 {
		dw.reconnectInitial = initial
	}
	if max >= initial && max > 0 {
		dw.reconnectMax = max
	}
}

func (dw *DockerGenerator) setConnected(connected bool) {
	if dw.onConnectionChange != nil {
		dw.onConnectionChange(connected)
	}
}

// Subscribe streams container events. It transparently reconnects (with bounded
// exponential backoff) if the Docker event stream drops, and only stops when
// ctx is cancelled. On each (re)connection it re-lists running containers so
// in-memory state re-syncs, and resumes the event stream from the last-seen
// event so transitions during the outage are not missed.
func (dw *DockerGenerator) Subscribe(ctx context.Context) (<-chan domain.ContainerEvent, error) {
	out := make(chan domain.ContainerEvent, dw.bufferSize)

	go func() {
		defer close(out)
		defer dw.setConnected(false)

		since := time.Now()
		backoff := dw.reconnectInitial

		for {
			if ctx.Err() != nil {
				return
			}

			err := dw.runOnce(ctx, out, &since)

			// A cancelled context is a clean shutdown, not a disconnect.
			if ctx.Err() != nil {
				return
			}

			dw.setConnected(false)
			if err != nil {
				dw.logger.Error().Err(err).Dur("backoff", backoff).Msg("Docker event stream failed; reconnecting after backoff")
			} else {
				dw.logger.Warn().Dur("backoff", backoff).Msg("Docker event stream closed; reconnecting after backoff")
			}

			if !sleepCtx(ctx, jitter(backoff)) {
				return
			}
			backoff = nextBackoff(backoff, dw.reconnectMax)
		}
	}()

	return out, nil
}

// runOnce performs a single connection cycle: it lists current containers, then
// streams events until the stream ends. It returns nil when the stream closed
// (caller should reconnect), an error when the connection failed, or simply
// returns when ctx is cancelled (caller detects this via ctx.Err()).
func (dw *DockerGenerator) runOnce(ctx context.Context, out chan<- domain.ContainerEvent, since *time.Time) error {
	containers, err := dw.cli.ContainerList(ctx, container.ListOptions{All: false})
	if err != nil {
		return fmt.Errorf("listing containers: %w", err)
	}
	for _, c := range containers {
		select {
		case out <- fromContainerSummary(c):
		case <-ctx.Done():
			return nil
		}
	}

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
	dw.setConnected(true)
	dw.logger.Info().Msg("Subscribed to Docker events")

	for {
		select {
		case <-ctx.Done():
			return nil
		case err, ok := <-errCh:
			if !ok {
				// Error channel closed; disable this case and keep reading events.
				errCh = nil
				continue
			}
			if err != nil {
				return fmt.Errorf("docker events stream: %w", err)
			}
		case msg, ok := <-eventCh:
			if !ok {
				// Event stream closed; signal a reconnect.
				return nil
			}
			// Track progress so a reconnect resumes from here.
			*since = time.Unix(0, msg.TimeNano)

			event, convErr := fromEventsMessage(msg)
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
				return nil
			}
		}
	}
}

// nextBackoff doubles the current backoff, capped at max.
func nextBackoff(cur, max time.Duration) time.Duration {
	next := cur * 2
	if next > max {
		return max
	}
	return next
}

// jitter returns d with up to +25% random jitter to avoid thundering-herd
// reconnects across multiple hosts.
func jitter(d time.Duration) time.Duration {
	if d <= 0 {
		return d
	}
	return d + time.Duration(rand.Int63n(int64(d)/4+1))
}

// sleepCtx sleeps for d, returning false if ctx is cancelled first.
func sleepCtx(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}
