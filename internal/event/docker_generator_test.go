package event

import (
	"context"
	"errors"
	"io"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/auto-dns/docker-coredns-sync/internal/domain"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/events"
	"github.com/rs/zerolog"
)

func testLogger() zerolog.Logger {
	return zerolog.New(io.Discard)
}

// fastGenerator returns a generator with tiny reconnect backoff so reconnect
// paths exercise quickly in tests.
func fastGenerator(cli dockerClient) *DockerGenerator {
	gen := NewDockerGenerator(cli, testLogger())
	gen.SetReconnectBackoff(2*time.Millisecond, 10*time.Millisecond)
	return gen
}

func startMsg(id, name string) events.Message {
	return events.Message{
		ID:       id,
		Status:   "start",
		TimeNano: time.Now().UnixNano(),
		Actor:    events.Actor{Attributes: map[string]string{"name": name}},
	}
}

func TestNewDockerGenerator(t *testing.T) {
	mock := newMockDockerClient()
	gen := NewDockerGenerator(mock, testLogger())

	if gen == nil {
		t.Fatal("expected non-nil generator")
	}
	if gen.cli != mock {
		t.Error("expected client to be set")
	}
	if gen.bufferSize != defaultEventBufferSize {
		t.Errorf("expected default buffer size %d, got %d", defaultEventBufferSize, gen.bufferSize)
	}
}

func TestDockerGenerator_Subscribe_ReturnsChannel(t *testing.T) {
	gen := fastGenerator(newMockDockerClient())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch, err := gen.Subscribe(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ch == nil {
		t.Fatal("expected non-nil channel")
	}
}

func TestDockerGenerator_Subscribe_EmitsInitialContainers(t *testing.T) {
	mock := newMockDockerClient()
	mock.containerListFunc = func(ctx context.Context, options container.ListOptions) ([]container.Summary, error) {
		return []container.Summary{
			{ID: "container-1", Names: []string{"/app-1"}, Created: time.Now().Unix()},
			{ID: "container-2", Names: []string{"/app-2"}, Created: time.Now().Unix()},
		}, nil
	}

	gen := fastGenerator(mock)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch, err := gen.Subscribe(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var received []domain.ContainerEvent
	for ev := range ch {
		received = append(received, ev)
		if len(received) == 2 {
			cancel()
		}
	}

	if len(received) != 2 {
		t.Fatalf("expected 2 events, got %d", len(received))
	}
	for _, ev := range received {
		if ev.EventType != domain.EventTypeInitialContainerDetection {
			t.Errorf("expected InitialContainerDetection, got %v", ev.EventType)
		}
	}
}

func TestDockerGenerator_Subscribe_ContainerListErrorRetries(t *testing.T) {
	var calls int32
	mock := newMockDockerClient()
	mock.containerListFunc = func(ctx context.Context, options container.ListOptions) ([]container.Summary, error) {
		atomic.AddInt32(&calls, 1)
		return nil, errors.New("docker daemon unavailable")
	}

	gen := fastGenerator(mock)
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()

	ch, err := gen.Subscribe(ctx)
	if err != nil {
		t.Fatalf("unexpected error from Subscribe: %v", err)
	}

	var received []domain.ContainerEvent
	for ev := range ch {
		received = append(received, ev)
	}

	if len(received) != 0 {
		t.Errorf("expected 0 events on error, got %d", len(received))
	}
	if atomic.LoadInt32(&calls) < 2 {
		t.Errorf("expected ContainerList to be retried at least twice, got %d calls", calls)
	}
}

func TestDockerGenerator_Subscribe_EmitsLiveEvents(t *testing.T) {
	eventCh := make(chan events.Message, 10)
	errCh := make(chan error)

	mock := newMockDockerClient()
	mock.eventsFunc = func(ctx context.Context, options events.ListOptions) (<-chan events.Message, <-chan error) {
		return eventCh, errCh
	}

	gen := fastGenerator(mock)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	ch, err := gen.Subscribe(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	go func() {
		time.Sleep(50 * time.Millisecond)
		eventCh <- startMsg("container-live", "live-app")
	}()

	var received []domain.ContainerEvent
	for ev := range ch {
		received = append(received, ev)
		cancel() // stop after the first live event
	}

	if len(received) != 1 {
		t.Fatalf("expected 1 event, got %d", len(received))
	}
	if received[0].Container.Id != "container-live" {
		t.Errorf("expected container Id 'container-live', got %q", received[0].Container.Id)
	}
	if received[0].EventType != domain.EventTypeContainerStarted {
		t.Errorf("expected EventType ContainerStarted, got %v", received[0].EventType)
	}
}

func TestDockerGenerator_Subscribe_FiltersEventTypes(t *testing.T) {
	cases := []struct {
		status string
		want   domain.EventType
	}{
		{"die", domain.EventTypeContainerDied},
		{"stop", domain.EventTypeContainerStopped},
	}
	for _, tc := range cases {
		t.Run(tc.status, func(t *testing.T) {
			eventCh := make(chan events.Message, 10)
			errCh := make(chan error)
			mock := newMockDockerClient()
			mock.eventsFunc = func(ctx context.Context, options events.ListOptions) (<-chan events.Message, <-chan error) {
				return eventCh, errCh
			}

			gen := fastGenerator(mock)
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()

			ch, err := gen.Subscribe(ctx)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			go func() {
				time.Sleep(50 * time.Millisecond)
				eventCh <- events.Message{
					ID:       "c1",
					Status:   tc.status,
					TimeNano: time.Now().UnixNano(),
					Actor:    events.Actor{Attributes: map[string]string{"name": "app"}},
				}
			}()

			var received []domain.ContainerEvent
			for ev := range ch {
				received = append(received, ev)
				cancel()
			}

			if len(received) != 1 {
				t.Fatalf("expected 1 event, got %d", len(received))
			}
			if received[0].EventType != tc.want {
				t.Errorf("expected %v, got %v", tc.want, received[0].EventType)
			}
		})
	}
}

func TestDockerGenerator_Subscribe_SkipsUnsupportedEvents(t *testing.T) {
	eventCh := make(chan events.Message, 10)
	errCh := make(chan error)
	mock := newMockDockerClient()
	mock.eventsFunc = func(ctx context.Context, options events.ListOptions) (<-chan events.Message, <-chan error) {
		return eventCh, errCh
	}

	gen := fastGenerator(mock)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	ch, err := gen.Subscribe(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	go func() {
		time.Sleep(50 * time.Millisecond)
		eventCh <- events.Message{ // unsupported
			ID: "c1", Status: "create", TimeNano: time.Now().UnixNano(),
			Actor: events.Actor{Attributes: map[string]string{"name": "app"}},
		}
		eventCh <- startMsg("c2", "app-2") // supported
	}()

	var received []domain.ContainerEvent
	for ev := range ch {
		received = append(received, ev)
		cancel()
	}

	if len(received) != 1 {
		t.Fatalf("expected 1 event (unsupported filtered), got %d", len(received))
	}
	if received[0].Container.Id != "c2" {
		t.Errorf("expected c2 (the start event), got %q", received[0].Container.Id)
	}
}

func TestDockerGenerator_Subscribe_ContextCancellation(t *testing.T) {
	mock := newMockDockerClient()
	mock.containerListFunc = func(ctx context.Context, options container.ListOptions) ([]container.Summary, error) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(5 * time.Second):
			return []container.Summary{}, nil
		}
	}

	gen := fastGenerator(mock)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	ch, err := gen.Subscribe(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	start := time.Now()
	for range ch {
	}
	if elapsed := time.Since(start); elapsed > 1*time.Second {
		t.Errorf("expected channel to close quickly, took %v", elapsed)
	}
}

func TestDockerGenerator_Subscribe_ErrorTriggersReconnect(t *testing.T) {
	var connects int32
	mock := newMockDockerClient()
	mock.eventsFunc = func(ctx context.Context, options events.ListOptions) (<-chan events.Message, <-chan error) {
		n := atomic.AddInt32(&connects, 1)
		eventCh := make(chan events.Message, 1)
		errCh := make(chan error, 1)
		if n == 1 {
			// First connection immediately errors out, forcing a reconnect.
			errCh <- errors.New("connection reset")
		} else {
			// Second connection delivers an event.
			eventCh <- startMsg("after-reconnect", "app")
		}
		return eventCh, errCh
	}

	gen := fastGenerator(mock)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	ch, err := gen.Subscribe(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var received []domain.ContainerEvent
	for ev := range ch {
		received = append(received, ev)
		cancel()
	}

	if len(received) != 1 {
		t.Fatalf("expected 1 event after reconnect, got %d", len(received))
	}
	if received[0].Container.Id != "after-reconnect" {
		t.Errorf("expected event from the second connection, got %q", received[0].Container.Id)
	}
	if atomic.LoadInt32(&connects) < 2 {
		t.Errorf("expected at least 2 connection attempts, got %d", connects)
	}
}

func TestDockerGenerator_Subscribe_EventChannelCloseTriggersReconnect(t *testing.T) {
	var connects int32
	mock := newMockDockerClient()
	mock.eventsFunc = func(ctx context.Context, options events.ListOptions) (<-chan events.Message, <-chan error) {
		n := atomic.AddInt32(&connects, 1)
		errCh := make(chan error)
		if n == 1 {
			eventCh := make(chan events.Message)
			close(eventCh) // closed stream -> reconnect
			return eventCh, errCh
		}
		eventCh := make(chan events.Message, 1)
		eventCh <- startMsg("after-close", "app")
		return eventCh, errCh
	}

	gen := fastGenerator(mock)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	ch, err := gen.Subscribe(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var received []domain.ContainerEvent
	for ev := range ch {
		received = append(received, ev)
		cancel()
	}

	if len(received) != 1 {
		t.Fatalf("expected 1 event after reconnect, got %d", len(received))
	}
	if received[0].Container.Id != "after-close" {
		t.Errorf("expected event from the second connection, got %q", received[0].Container.Id)
	}
}

func TestDockerGenerator_Subscribe_ErrorChannelCloseDoesNotReconnect(t *testing.T) {
	eventCh := make(chan events.Message, 1)
	errCh := make(chan error)
	var connects int32

	mock := newMockDockerClient()
	mock.eventsFunc = func(ctx context.Context, options events.ListOptions) (<-chan events.Message, <-chan error) {
		atomic.AddInt32(&connects, 1)
		return eventCh, errCh
	}

	gen := fastGenerator(mock)
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	ch, err := gen.Subscribe(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	go func() {
		time.Sleep(50 * time.Millisecond)
		close(errCh) // closing the error channel must NOT cause a reconnect
		eventCh <- startMsg("still-here", "app")
	}()

	var received []domain.ContainerEvent
	for ev := range ch {
		received = append(received, ev)
		cancel()
	}

	if len(received) != 1 {
		t.Fatalf("expected 1 event, got %d", len(received))
	}
	if c := atomic.LoadInt32(&connects); c != 1 {
		t.Errorf("expected a single connection (no reconnect on errCh close), got %d", c)
	}
}

func TestDockerGenerator_Subscribe_ConnectionObserver(t *testing.T) {
	eventCh := make(chan events.Message)
	errCh := make(chan error)
	mock := newMockDockerClient()
	mock.eventsFunc = func(ctx context.Context, options events.ListOptions) (<-chan events.Message, <-chan error) {
		return eventCh, errCh
	}

	var mu sync.Mutex
	var states []bool
	gen := fastGenerator(mock)
	gen.SetConnectionObserver(func(connected bool) {
		mu.Lock()
		states = append(states, connected)
		mu.Unlock()
	})

	ctx, cancel := context.WithCancel(context.Background())

	ch, err := gen.Subscribe(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Give it a moment to connect, then cancel.
	time.Sleep(50 * time.Millisecond)
	cancel()
	for range ch {
	}

	mu.Lock()
	defer mu.Unlock()
	if len(states) == 0 || states[0] != true {
		t.Fatalf("expected first observed state to be connected=true, got %v", states)
	}
	if states[len(states)-1] != false {
		t.Errorf("expected final observed state to be connected=false, got %v", states)
	}
}

func TestDockerGenerator_Subscribe_ContextCancelDuringInitialEmit(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	mock := newMockDockerClient()
	mock.containerListFunc = func(ctx context.Context, options container.ListOptions) ([]container.Summary, error) {
		containers := make([]container.Summary, 100)
		for i := 0; i < 100; i++ {
			containers[i] = container.Summary{
				ID:      "container-" + string(rune('a'+i%26)),
				Names:   []string{"/container-" + string(rune('a'+i%26))},
				Created: time.Now().Unix(),
			}
		}
		return containers, nil
	}

	gen := fastGenerator(mock)
	ch, err := gen.Subscribe(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cancel()

	count := 0
	for range ch {
		count++
	}
	t.Logf("received %d events before cancellation", count)
}

func TestNextBackoff(t *testing.T) {
	if got := nextBackoff(10*time.Millisecond, 100*time.Millisecond); got != 20*time.Millisecond {
		t.Errorf("expected 20ms, got %v", got)
	}
	if got := nextBackoff(80*time.Millisecond, 100*time.Millisecond); got != 100*time.Millisecond {
		t.Errorf("expected cap at 100ms, got %v", got)
	}
}

func TestJitter(t *testing.T) {
	d := 100 * time.Millisecond
	for i := 0; i < 100; i++ {
		j := jitter(d)
		if j < d || j > d+d/4 {
			t.Fatalf("jitter out of range: %v (want [%v, %v])", j, d, d+d/4)
		}
	}
	if jitter(0) != 0 {
		t.Error("expected jitter(0) == 0")
	}
}
