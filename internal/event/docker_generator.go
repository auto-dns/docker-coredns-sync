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

// Defaults used when no option overrides them. These mirror the viper defaults
// in internal/config (docker.event_buffer_size / reconnect_*); keep them in
// sync.
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

// Option configures a DockerGenerator at construction time.
type Option func(*DockerGenerator)

// WithEventBufferSize sets the size of the buffered output channel.
func WithEventBufferSize(n int) Option {
	return func(dw *DockerGenerator) {
		if n > 0 {
			dw.bufferSize = n
		}
	}
}

// WithReconnectBackoff sets the reconnect backoff bounds. max is clamped up to
// initial so the bounds are always consistent even if called with max < initial.
func WithReconnectBackoff(initial, max time.Duration) Option {
	return func(dw *DockerGenerator) {
		if initial > 0 {
			dw.reconnectInitial = initial
		}
		if max > 0 {
			dw.reconnectMax = max
		}
		if dw.reconnectMax < dw.reconnectInitial {
			dw.reconnectMax = dw.reconnectInitial
		}
	}
}

// WithConnectionObserver registers a callback invoked when the Docker event
// stream connects (true) or disconnects (false). The callback must be
// non-blocking: it runs inline on the generator's goroutine.
func WithConnectionObserver(fn func(connected bool)) Option {
	return func(dw *DockerGenerator) {
		dw.onConnectionChange = fn
	}
}

func NewDockerGenerator(cli dockerClient, logger zerolog.Logger, opts ...Option) *DockerGenerator {
	dw := &DockerGenerator{
		logger:           logger,
		cli:              cli,
		bufferSize:       defaultEventBufferSize,
		reconnectInitial: defaultReconnectInitial,
		reconnectMax:     defaultReconnectMaxBackoff,
	}
	for _, opt := range opts {
		opt(dw)
	}
	return dw
}

func (dw *DockerGenerator) setConnected(connected bool) {
	if dw.onConnectionChange != nil {
		dw.onConnectionChange(connected)
	}
}

// Subscribe streams container events. It transparently reconnects (with bounded
// exponential backoff) if the Docker event stream drops, and only stops when
// ctx is cancelled. On each (re)connection it re-lists running containers so
// in-memory state re-syncs (including pruning containers that stopped while
// disconnected, via a resync event), and resumes the event stream from the
// last-seen event so transitions during the outage are not missed.
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

			connectedAt, err := dw.runOnce(ctx, out, &since)

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

			// Only reset the backoff after a connection that stayed up long
			// enough to be considered healthy. A connection that connects then
			// drops immediately keeps escalating the backoff, so a flapping
			// daemon does not turn into a tight reconnect loop.
			if stableConnection(connectedAt, time.Now(), dw.reconnectMax) {
				backoff = dw.reconnectInitial
			}

			if !sleepCtx(ctx, jitter(backoff)) {
				return
			}
			backoff = nextBackoff(backoff, dw.reconnectMax)
		}
	}()

	return out, nil
}

// runOnce performs a single connection cycle: it lists current containers
// (emitting detection events plus a resync event), then streams events until
// the stream ends. It returns the time the event stream connected (zero if it
// never connected) and an error when the connection failed. It returns when
// ctx is cancelled (caller detects this via ctx.Err()).
func (dw *DockerGenerator) runOnce(ctx context.Context, out chan<- domain.ContainerEvent, since *time.Time) (time.Time, error) {
	var notConnected time.Time
	containers, err := dw.cli.ContainerList(ctx, container.ListOptions{All: false})
	if err != nil {
		return notConnected, fmt.Errorf("listing containers: %w", err)
	}
	runningIds := make([]string, 0, len(containers))
	for _, c := range containers {
		runningIds = append(runningIds, c.ID)
		select {
		case out <- fromContainerSummary(c):
		case <-ctx.Done():
			return notConnected, nil
		}
	}
	// Tell the engine the full running set so it can prune containers that
	// stopped while we were disconnected.
	select {
	case out <- domain.ContainerEvent{EventType: domain.EventTypeResync, RunningContainerIds: runningIds}:
	case <-ctx.Done():
		return notConnected, nil
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
	connectedAt := time.Now()
	dw.setConnected(true)
	dw.logger.Info().Msg("Subscribed to Docker events")

	for {
		select {
		case <-ctx.Done():
			return connectedAt, nil
		case err, ok := <-errCh:
			if !ok {
				// The client closed the error channel; treat the stream as
				// ended and reconnect rather than risk reading a now-dead
				// event channel forever.
				return connectedAt, nil
			}
			if err != nil {
				return connectedAt, fmt.Errorf("docker events stream: %w", err)
			}
		case msg, ok := <-eventCh:
			if !ok {
				// Event stream closed; signal a reconnect.
				return connectedAt, nil
			}
			// Track progress so a reconnect resumes from here. Guard against a
			// zero/absent timestamp, which would otherwise rewind `since` to
			// the epoch and replay the entire event history on reconnect.
			if msg.TimeNano > 0 {
				*since = time.Unix(0, msg.TimeNano)
			}

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
				return connectedAt, nil
			}
		}
	}
}

// stableConnection reports whether a connection established at connectedAt was
// up long enough (>= minUptime) to be considered healthy. A zero connectedAt
// means the connection was never established.
func stableConnection(connectedAt, now time.Time, minUptime time.Duration) bool {
	if connectedAt.IsZero() {
		return false
	}
	return now.Sub(connectedAt) >= minUptime
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
