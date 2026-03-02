package event

import (
	"context"
	"errors"
	"io"
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

func TestNewDockerGenerator(t *testing.T) {
	mock := newMockDockerClient()
	logger := testLogger()

	gen := NewDockerGenerator(mock, logger)

	if gen == nil {
		t.Fatal("expected non-nil generator")
	}
	if gen.cli != mock {
		t.Error("expected client to be set")
	}
}

func TestDockerGenerator_Subscribe_ReturnsChannel(t *testing.T) {
	mock := newMockDockerClient()
	gen := NewDockerGenerator(mock, testLogger())

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
			{
				ID:      "container-1",
				Names:   []string{"/app-1"},
				Created: time.Now().Unix(),
				Labels: map[string]string{
					"coredns.enabled": "true",
				},
			},
			{
				ID:      "container-2",
				Names:   []string{"/app-2"},
				Created: time.Now().Unix(),
				Labels: map[string]string{
					"coredns.enabled": "true",
				},
			},
		}, nil
	}

	gen := NewDockerGenerator(mock, testLogger())

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
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

	// Both should be initial detection events
	for _, ev := range received {
		if ev.EventType != domain.EventTypeInitialContainerDetection {
			t.Errorf("expected InitialContainerDetection, got %v", ev.EventType)
		}
	}

	if received[0].Container.Id != "container-1" {
		t.Errorf("expected first container to be 'container-1', got %q", received[0].Container.Id)
	}
	if received[1].Container.Id != "container-2" {
		t.Errorf("expected second container to be 'container-2', got %q", received[1].Container.Id)
	}
}

func TestDockerGenerator_Subscribe_ContainerListError(t *testing.T) {
	mock := newMockDockerClient()
	mock.containerListFunc = func(ctx context.Context, options container.ListOptions) ([]container.Summary, error) {
		return nil, errors.New("docker daemon unavailable")
	}

	gen := NewDockerGenerator(mock, testLogger())

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	ch, err := gen.Subscribe(ctx)
	if err != nil {
		t.Fatalf("unexpected error from Subscribe: %v", err)
	}

	// Channel should be closed without emitting events
	var received []domain.ContainerEvent
	for ev := range ch {
		received = append(received, ev)
	}

	if len(received) != 0 {
		t.Errorf("expected 0 events on error, got %d", len(received))
	}
}

func TestDockerGenerator_Subscribe_EmitsLiveEvents(t *testing.T) {
	eventCh := make(chan events.Message, 10)
	errCh := make(chan error)

	mock := newMockDockerClient()
	mock.containerListFunc = func(ctx context.Context, options container.ListOptions) ([]container.Summary, error) {
		return []container.Summary{}, nil
	}
	mock.eventsFunc = func(ctx context.Context, options events.ListOptions) (<-chan events.Message, <-chan error) {
		return eventCh, errCh
	}

	gen := NewDockerGenerator(mock, testLogger())

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	ch, err := gen.Subscribe(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Send a start event
	go func() {
		time.Sleep(100 * time.Millisecond) // let generator set up
		eventCh <- events.Message{
			ID:       "container-live",
			Status:   "start",
			TimeNano: time.Now().UnixNano(),
			Actor: events.Actor{
				Attributes: map[string]string{
					"name": "live-app",
				},
			},
		}
		time.Sleep(100 * time.Millisecond)
		close(eventCh)
	}()

	var received []domain.ContainerEvent
	for ev := range ch {
		received = append(received, ev)
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

func TestDockerGenerator_Subscribe_FiltersDieEvent(t *testing.T) {
	eventCh := make(chan events.Message, 10)
	errCh := make(chan error)

	mock := newMockDockerClient()
	mock.containerListFunc = func(ctx context.Context, options container.ListOptions) ([]container.Summary, error) {
		return []container.Summary{}, nil
	}
	mock.eventsFunc = func(ctx context.Context, options events.ListOptions) (<-chan events.Message, <-chan error) {
		return eventCh, errCh
	}

	gen := NewDockerGenerator(mock, testLogger())

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	ch, err := gen.Subscribe(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	go func() {
		time.Sleep(100 * time.Millisecond)
		eventCh <- events.Message{
			ID:       "dying-container",
			Status:   "die",
			TimeNano: time.Now().UnixNano(),
			Actor: events.Actor{
				Attributes: map[string]string{
					"name": "dying-app",
				},
			},
		}
		time.Sleep(100 * time.Millisecond)
		close(eventCh)
	}()

	var received []domain.ContainerEvent
	for ev := range ch {
		received = append(received, ev)
	}

	if len(received) != 1 {
		t.Fatalf("expected 1 event, got %d", len(received))
	}

	if received[0].EventType != domain.EventTypeContainerDied {
		t.Errorf("expected EventType ContainerDied, got %v", received[0].EventType)
	}
}

func TestDockerGenerator_Subscribe_SkipsUnsupportedEvents(t *testing.T) {
	eventCh := make(chan events.Message, 10)
	errCh := make(chan error)

	mock := newMockDockerClient()
	mock.containerListFunc = func(ctx context.Context, options container.ListOptions) ([]container.Summary, error) {
		return []container.Summary{}, nil
	}
	mock.eventsFunc = func(ctx context.Context, options events.ListOptions) (<-chan events.Message, <-chan error) {
		return eventCh, errCh
	}

	gen := NewDockerGenerator(mock, testLogger())

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	ch, err := gen.Subscribe(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	go func() {
		time.Sleep(100 * time.Millisecond)
		// Send unsupported event
		eventCh <- events.Message{
			ID:       "container-1",
			Status:   "create", // unsupported
			TimeNano: time.Now().UnixNano(),
			Actor: events.Actor{
				Attributes: map[string]string{"name": "app"},
			},
		}
		// Send supported event
		eventCh <- events.Message{
			ID:       "container-2",
			Status:   "start",
			TimeNano: time.Now().UnixNano(),
			Actor: events.Actor{
				Attributes: map[string]string{"name": "app-2"},
			},
		}
		time.Sleep(100 * time.Millisecond)
		close(eventCh)
	}()

	var received []domain.ContainerEvent
	for ev := range ch {
		received = append(received, ev)
	}

	// Only the start event should come through
	if len(received) != 1 {
		t.Fatalf("expected 1 event (unsupported filtered), got %d", len(received))
	}

	if received[0].Container.Id != "container-2" {
		t.Errorf("expected container-2 (the start event), got %q", received[0].Container.Id)
	}
}

func TestDockerGenerator_Subscribe_ContextCancellation(t *testing.T) {
	mock := newMockDockerClient()
	mock.containerListFunc = func(ctx context.Context, options container.ListOptions) ([]container.Summary, error) {
		// Simulate slow container list
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(5 * time.Second):
			return []container.Summary{}, nil
		}
	}

	gen := NewDockerGenerator(mock, testLogger())

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	ch, err := gen.Subscribe(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Channel should close quickly due to context cancellation
	start := time.Now()
	for range ch {
		// drain
	}
	elapsed := time.Since(start)

	if elapsed > 1*time.Second {
		t.Errorf("expected channel to close quickly, took %v", elapsed)
	}
}

func TestDockerGenerator_Subscribe_MultipleEvents(t *testing.T) {
	eventCh := make(chan events.Message, 10)
	errCh := make(chan error)

	mock := newMockDockerClient()
	mock.containerListFunc = func(ctx context.Context, options container.ListOptions) ([]container.Summary, error) {
		return []container.Summary{
			{ID: "initial-1", Names: []string{"/initial"}, Created: time.Now().Unix()},
		}, nil
	}
	mock.eventsFunc = func(ctx context.Context, options events.ListOptions) (<-chan events.Message, <-chan error) {
		return eventCh, errCh
	}

	gen := NewDockerGenerator(mock, testLogger())

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	ch, err := gen.Subscribe(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	go func() {
		time.Sleep(100 * time.Millisecond)
		eventCh <- events.Message{
			ID: "live-1", Status: "start", TimeNano: time.Now().UnixNano(),
			Actor: events.Actor{Attributes: map[string]string{"name": "live-1"}},
		}
		eventCh <- events.Message{
			ID: "live-2", Status: "start", TimeNano: time.Now().UnixNano(),
			Actor: events.Actor{Attributes: map[string]string{"name": "live-2"}},
		}
		eventCh <- events.Message{
			ID: "live-1", Status: "die", TimeNano: time.Now().UnixNano(),
			Actor: events.Actor{Attributes: map[string]string{"name": "live-1"}},
		}
		time.Sleep(100 * time.Millisecond)
		close(eventCh)
	}()

	var received []domain.ContainerEvent
	for ev := range ch {
		received = append(received, ev)
	}

	// 1 initial + 3 live events = 4 total
	if len(received) != 4 {
		t.Fatalf("expected 4 events, got %d", len(received))
	}

	// First should be initial detection
	if received[0].EventType != domain.EventTypeInitialContainerDetection {
		t.Errorf("expected first event to be InitialContainerDetection, got %v", received[0].EventType)
	}

	// Count event types
	startCount := 0
	dieCount := 0
	for _, ev := range received[1:] {
		switch ev.EventType {
		case domain.EventTypeContainerStarted:
			startCount++
		case domain.EventTypeContainerDied:
			dieCount++
		}
	}

	if startCount != 2 {
		t.Errorf("expected 2 start events, got %d", startCount)
	}
	if dieCount != 1 {
		t.Errorf("expected 1 die event, got %d", dieCount)
	}
}

func TestDockerGenerator_Subscribe_ErrorChannelHandled(t *testing.T) {
	eventCh := make(chan events.Message, 10)
	errCh := make(chan error, 10)

	mock := newMockDockerClient()
	mock.containerListFunc = func(ctx context.Context, options container.ListOptions) ([]container.Summary, error) {
		return []container.Summary{}, nil
	}
	mock.eventsFunc = func(ctx context.Context, options events.ListOptions) (<-chan events.Message, <-chan error) {
		return eventCh, errCh
	}

	gen := NewDockerGenerator(mock, testLogger())

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	ch, err := gen.Subscribe(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	go func() {
		time.Sleep(100 * time.Millisecond)
		// Send an error
		errCh <- errors.New("connection reset")
		// Then a valid event
		eventCh <- events.Message{
			ID: "after-error", Status: "start", TimeNano: time.Now().UnixNano(),
			Actor: events.Actor{Attributes: map[string]string{"name": "after"}},
		}
		time.Sleep(100 * time.Millisecond)
		close(eventCh)
	}()

	var received []domain.ContainerEvent
	for ev := range ch {
		received = append(received, ev)
	}

	// Should still receive the event after the error
	if len(received) != 1 {
		t.Fatalf("expected 1 event after error handling, got %d", len(received))
	}
}

func TestDockerGenerator_Subscribe_StopEvent(t *testing.T) {
	eventCh := make(chan events.Message, 10)
	errCh := make(chan error)

	mock := newMockDockerClient()
	mock.containerListFunc = func(ctx context.Context, options container.ListOptions) ([]container.Summary, error) {
		return []container.Summary{}, nil
	}
	mock.eventsFunc = func(ctx context.Context, options events.ListOptions) (<-chan events.Message, <-chan error) {
		return eventCh, errCh
	}

	gen := NewDockerGenerator(mock, testLogger())

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	ch, err := gen.Subscribe(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	go func() {
		time.Sleep(100 * time.Millisecond)
		eventCh <- events.Message{
			ID:       "stopping-container",
			Status:   "stop",
			TimeNano: time.Now().UnixNano(),
			Actor: events.Actor{
				Attributes: map[string]string{
					"name": "stopping-app",
				},
			},
		}
		time.Sleep(100 * time.Millisecond)
		close(eventCh)
	}()

	var received []domain.ContainerEvent
	for ev := range ch {
		received = append(received, ev)
	}

	if len(received) != 1 {
		t.Fatalf("expected 1 event, got %d", len(received))
	}

	if received[0].EventType != domain.EventTypeContainerStopped {
		t.Errorf("expected EventType ContainerStopped, got %v", received[0].EventType)
	}
}

func TestDockerGenerator_Subscribe_ContextCancelDuringInitialEmit(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	mock := newMockDockerClient()
	mock.containerListFunc = func(ctx context.Context, options container.ListOptions) ([]container.Summary, error) {
		// Return many containers to increase chance of hitting the cancel
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
	mock.eventsFunc = func(ctx context.Context, options events.ListOptions) (<-chan events.Message, <-chan error) {
		msgCh := make(chan events.Message)
		errCh := make(chan error)
		return msgCh, errCh
	}

	gen := NewDockerGenerator(mock, testLogger())

	ch, err := gen.Subscribe(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Cancel immediately to trigger the context cancellation during emit
	cancel()

	// Drain the channel
	count := 0
	for range ch {
		count++
	}

	// Should have received fewer than 100 (cancelled during emit) or possibly all 100 if fast enough
	// The key is that it doesn't hang and the channel closes properly
	t.Logf("received %d events before cancellation", count)
}

func TestDockerGenerator_Subscribe_ContextCancelDuringEventSend(t *testing.T) {
	eventCh := make(chan events.Message, 10)
	errCh := make(chan error)

	mock := newMockDockerClient()
	mock.containerListFunc = func(ctx context.Context, options container.ListOptions) ([]container.Summary, error) {
		return []container.Summary{}, nil
	}
	mock.eventsFunc = func(ctx context.Context, options events.ListOptions) (<-chan events.Message, <-chan error) {
		return eventCh, errCh
	}

	gen := NewDockerGenerator(mock, testLogger())

	ctx, cancel := context.WithCancel(context.Background())

	ch, err := gen.Subscribe(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Send events but don't consume them - causes blocking on send
	go func() {
		time.Sleep(50 * time.Millisecond)
		// Fill the output buffer
		for i := 0; i < 150; i++ {
			select {
			case eventCh <- events.Message{
				ID:       "container",
				Status:   "start",
				TimeNano: time.Now().UnixNano(),
				Actor:    events.Actor{Attributes: map[string]string{"name": "app"}},
			}:
			case <-time.After(10 * time.Millisecond):
				// Event channel might be full
			}
		}
	}()

	// Cancel while events are being sent
	time.Sleep(100 * time.Millisecond)
	cancel()

	// Drain the channel - should close due to context cancellation
	count := 0
	for range ch {
		count++
	}

	t.Logf("received %d events before context cancellation", count)
}

func TestDockerGenerator_Subscribe_ErrorChannelClosed(t *testing.T) {
	eventCh := make(chan events.Message, 10)
	errCh := make(chan error)

	mock := newMockDockerClient()
	mock.containerListFunc = func(ctx context.Context, options container.ListOptions) ([]container.Summary, error) {
		return []container.Summary{}, nil
	}
	mock.eventsFunc = func(ctx context.Context, options events.ListOptions) (<-chan events.Message, <-chan error) {
		return eventCh, errCh
	}

	gen := NewDockerGenerator(mock, testLogger())

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	ch, err := gen.Subscribe(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	go func() {
		time.Sleep(50 * time.Millisecond)
		// Close error channel (simulates Docker client cleanup)
		close(errCh)
		// Send one more event
		eventCh <- events.Message{
			ID:       "container",
			Status:   "start",
			TimeNano: time.Now().UnixNano(),
			Actor:    events.Actor{Attributes: map[string]string{"name": "app"}},
		}
		time.Sleep(50 * time.Millisecond)
		close(eventCh)
	}()

	var received []domain.ContainerEvent
	for ev := range ch {
		received = append(received, ev)
	}

	if len(received) != 1 {
		t.Errorf("expected 1 event, got %d", len(received))
	}
}

func TestDockerGenerator_Subscribe_ErrorChannelReceivesError(t *testing.T) {
	eventCh := make(chan events.Message, 10)
	errCh := make(chan error, 10)

	mock := newMockDockerClient()
	mock.containerListFunc = func(ctx context.Context, options container.ListOptions) ([]container.Summary, error) {
		return []container.Summary{}, nil
	}
	mock.eventsFunc = func(ctx context.Context, options events.ListOptions) (<-chan events.Message, <-chan error) {
		return eventCh, errCh
	}

	gen := NewDockerGenerator(mock, testLogger())

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	ch, err := gen.Subscribe(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	go func() {
		time.Sleep(50 * time.Millisecond)
		// Send an error to the error channel
		errCh <- errors.New("docker daemon connection lost")
		// Send an event after the error to verify processing continues
		time.Sleep(50 * time.Millisecond)
		eventCh <- events.Message{
			ID:       "container-after-error",
			Status:   "start",
			TimeNano: time.Now().UnixNano(),
			Actor:    events.Actor{Attributes: map[string]string{"name": "app"}},
		}
		time.Sleep(50 * time.Millisecond)
		close(eventCh)
	}()

	var received []domain.ContainerEvent
	for ev := range ch {
		received = append(received, ev)
	}

	// Should still receive the event after the error
	if len(received) != 1 {
		t.Errorf("expected 1 event after error, got %d", len(received))
	}
}

func TestDockerGenerator_Subscribe_EventChannelClosed(t *testing.T) {
	eventCh := make(chan events.Message)
	errCh := make(chan error)

	mock := newMockDockerClient()
	mock.containerListFunc = func(ctx context.Context, options container.ListOptions) ([]container.Summary, error) {
		return []container.Summary{}, nil
	}
	mock.eventsFunc = func(ctx context.Context, options events.ListOptions) (<-chan events.Message, <-chan error) {
		return eventCh, errCh
	}

	gen := NewDockerGenerator(mock, testLogger())

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	ch, err := gen.Subscribe(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	go func() {
		time.Sleep(50 * time.Millisecond)
		// Immediately close the event channel (simulates Docker daemon shutdown)
		close(eventCh)
	}()

	// Channel should close cleanly
	var received []domain.ContainerEvent
	for ev := range ch {
		received = append(received, ev)
	}

	if len(received) != 0 {
		t.Errorf("expected 0 events, got %d", len(received))
	}
}
