package event

import (
	"context"
	"errors"
	"io"
	"strings"
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
func fastGenerator(cli dockerClient, opts ...Option) *DockerGenerator {
	base := []Option{WithReconnectBackoff(2*time.Millisecond, 10*time.Millisecond)}
	return NewDockerGenerator(cli, testLogger(), append(base, opts...)...)
}

func startMsg(id, name string) events.Message {
	return events.Message{
		ID:       id,
		Status:   "start",
		TimeNano: time.Now().UnixNano(),
		Actor:    events.Actor{Attributes: map[string]string{"name": name}},
	}
}

// isContainerEvent reports whether ev is a real container event (not a resync
// bookkeeping event).
func isContainerEvent(ev domain.ContainerEvent) bool {
	return ev.EventType != domain.EventTypeResync
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

func TestWithReconnectBackoff_ClampsMax(t *testing.T) {
	gen := NewDockerGenerator(newMockDockerClient(), testLogger(),
		WithReconnectBackoff(60*time.Second, 30*time.Second))
	if gen.reconnectMax < gen.reconnectInitial {
		t.Errorf("expected max (%v) to be clamped up to initial (%v)", gen.reconnectMax, gen.reconnectInitial)
	}
}

func TestWithEventBufferSize(t *testing.T) {
	gen := NewDockerGenerator(newMockDockerClient(), testLogger(), WithEventBufferSize(7))
	if gen.bufferSize != 7 {
		t.Errorf("expected buffer size 7, got %d", gen.bufferSize)
	}
	// Non-positive is ignored.
	gen = NewDockerGenerator(newMockDockerClient(), testLogger(), WithEventBufferSize(0))
	if gen.bufferSize != defaultEventBufferSize {
		t.Errorf("expected default buffer size for non-positive input, got %d", gen.bufferSize)
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
		if !isContainerEvent(ev) {
			continue
		}
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

func TestDockerGenerator_Subscribe_EmitsResyncEvent(t *testing.T) {
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

	var resync *domain.ContainerEvent
	for ev := range ch {
		if ev.EventType == domain.EventTypeResync {
			e := ev
			resync = &e
			cancel()
		}
	}

	if resync == nil {
		t.Fatal("expected a resync event")
	}
	if len(resync.RunningContainerIds) != 2 {
		t.Fatalf("expected 2 running ids, got %d", len(resync.RunningContainerIds))
	}
	ids := map[string]bool{}
	for _, id := range resync.RunningContainerIds {
		ids[id] = true
	}
	if !ids["container-1"] || !ids["container-2"] {
		t.Errorf("expected running ids to contain container-1 and container-2, got %v", resync.RunningContainerIds)
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
		if isContainerEvent(ev) {
			received = append(received, ev)
		}
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
		if !isContainerEvent(ev) {
			continue
		}
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
				if !isContainerEvent(ev) {
					continue
				}
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
		if !isContainerEvent(ev) {
			continue
		}
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

func TestDockerGenerator_Subscribe_MultipleEventsOrdered(t *testing.T) {
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
		eventCh <- startMsg("c1", "app-1")
		eventCh <- startMsg("c2", "app-2")
		eventCh <- events.Message{
			ID: "c1", Status: "die", TimeNano: time.Now().UnixNano(),
			Actor: events.Actor{Attributes: map[string]string{"name": "app-1"}},
		}
	}()

	var received []domain.ContainerEvent
	for ev := range ch {
		if !isContainerEvent(ev) {
			continue
		}
		received = append(received, ev)
		if len(received) == 3 {
			cancel()
		}
	}

	if len(received) != 3 {
		t.Fatalf("expected 3 events in order, got %d", len(received))
	}
	if received[0].Container.Id != "c1" || received[0].EventType != domain.EventTypeContainerStarted {
		t.Errorf("event 0 wrong: %+v", received[0])
	}
	if received[1].Container.Id != "c2" || received[1].EventType != domain.EventTypeContainerStarted {
		t.Errorf("event 1 wrong: %+v", received[1])
	}
	if received[2].Container.Id != "c1" || received[2].EventType != domain.EventTypeContainerDied {
		t.Errorf("event 2 wrong: %+v", received[2])
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

	// Small buffer so the initial emit loop blocks and must observe cancellation.
	gen := fastGenerator(mock, WithEventBufferSize(1))
	ch, err := gen.Subscribe(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cancel()

	done := make(chan struct{})
	go func() {
		for range ch {
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("channel did not close after cancellation during initial emit")
	}
}

func TestDockerGenerator_Subscribe_ContextCancelDuringEventSend(t *testing.T) {
	eventCh := make(chan events.Message, 100)
	errCh := make(chan error)
	mock := newMockDockerClient()
	mock.eventsFunc = func(ctx context.Context, options events.ListOptions) (<-chan events.Message, <-chan error) {
		return eventCh, errCh
	}

	// Buffer of 1; produce many events without consuming so the out-send blocks.
	gen := fastGenerator(mock, WithEventBufferSize(1))
	ctx, cancel := context.WithCancel(context.Background())

	ch, err := gen.Subscribe(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	go func() {
		for i := 0; i < 50; i++ {
			eventCh <- startMsg("c", "app")
		}
	}()

	time.Sleep(100 * time.Millisecond)
	cancel()

	done := make(chan struct{})
	go func() {
		for range ch {
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("channel did not close after cancellation during event send")
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
			errCh <- errors.New("connection reset") // force a reconnect
		} else {
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
		if !isContainerEvent(ev) {
			continue
		}
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
		if !isContainerEvent(ev) {
			continue
		}
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

func TestDockerGenerator_Subscribe_ErrorChannelCloseTriggersReconnect(t *testing.T) {
	var connects int32
	mock := newMockDockerClient()
	mock.eventsFunc = func(ctx context.Context, options events.ListOptions) (<-chan events.Message, <-chan error) {
		n := atomic.AddInt32(&connects, 1)
		eventCh := make(chan events.Message, 1)
		errCh := make(chan error)
		if n == 1 {
			close(errCh) // error channel closed -> reconnect (avoid silent stall)
		} else {
			eventCh <- startMsg("after-errclose", "app")
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
		if !isContainerEvent(ev) {
			continue
		}
		received = append(received, ev)
		cancel()
	}

	if len(received) != 1 {
		t.Fatalf("expected 1 event after reconnect, got %d", len(received))
	}
	if received[0].Container.Id != "after-errclose" {
		t.Errorf("expected event from the second connection, got %q", received[0].Container.Id)
	}
	if atomic.LoadInt32(&connects) < 2 {
		t.Errorf("expected reconnect after errCh close, got %d connects", connects)
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
	gen := fastGenerator(mock, WithConnectionObserver(func(connected bool) {
		mu.Lock()
		states = append(states, connected)
		mu.Unlock()
	}))

	ctx, cancel := context.WithCancel(context.Background())

	ch, err := gen.Subscribe(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

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

func TestStableConnection(t *testing.T) {
	now := time.Now()
	if stableConnection(time.Time{}, now, time.Second) {
		t.Error("a never-connected (zero) time should not be stable")
	}
	if !stableConnection(now.Add(-2*time.Second), now, time.Second) {
		t.Error("a connection up for 2s should be stable with a 1s threshold")
	}
	if stableConnection(now.Add(-500*time.Millisecond), now, time.Second) {
		t.Error("a connection up for 500ms should not be stable with a 1s threshold")
	}
}

func TestDockerGenerator_Subscribe_ResyncOnEachReconnect(t *testing.T) {
	var connects int32
	mock := newMockDockerClient()
	mock.containerListFunc = func(ctx context.Context, options container.ListOptions) ([]container.Summary, error) {
		return []container.Summary{{ID: "c1", Names: []string{"/c1"}, Created: time.Now().Unix()}}, nil
	}
	mock.eventsFunc = func(ctx context.Context, options events.ListOptions) (<-chan events.Message, <-chan error) {
		n := atomic.AddInt32(&connects, 1)
		errCh := make(chan error)
		if n == 1 {
			eventCh := make(chan events.Message)
			close(eventCh) // first stream drops -> reconnect
			return eventCh, errCh
		}
		return make(chan events.Message), errCh // second stays open
	}

	gen := fastGenerator(mock)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	ch, err := gen.Subscribe(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	resyncCount := 0
	for ev := range ch {
		if ev.EventType == domain.EventTypeResync {
			resyncCount++
			if resyncCount == 2 {
				cancel()
			}
		}
	}

	if resyncCount < 2 {
		t.Errorf("expected a resync event on each (re)connection, got %d", resyncCount)
	}
}

func TestDockerGenerator_Subscribe_ConnectionObserverReconnectCycle(t *testing.T) {
	var connects int32
	mock := newMockDockerClient()
	mock.eventsFunc = func(ctx context.Context, options events.ListOptions) (<-chan events.Message, <-chan error) {
		n := atomic.AddInt32(&connects, 1)
		errCh := make(chan error)
		if n == 1 {
			eventCh := make(chan events.Message)
			close(eventCh) // drop the first connection -> reconnect
			return eventCh, errCh
		}
		return make(chan events.Message), errCh // stays open
	}

	var mu sync.Mutex
	var states []bool
	gen := fastGenerator(mock, WithConnectionObserver(func(connected bool) {
		mu.Lock()
		states = append(states, connected)
		mu.Unlock()
	}))

	ctx, cancel := context.WithCancel(context.Background())
	ch, err := gen.Subscribe(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Wait for the reconnect to happen, then cancel.
	for i := 0; i < 200; i++ {
		if atomic.LoadInt32(&connects) >= 2 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	cancel()
	for range ch {
	}

	// Expect at least true -> false -> true across the disconnect/reconnect.
	mu.Lock()
	defer mu.Unlock()
	if !containsSubsequence(states, []bool{true, false, true}) {
		t.Errorf("expected observer to see true->false->true across reconnect, got %v", states)
	}
}

func containsSubsequence(haystack, needle []bool) bool {
	i := 0
	for _, v := range haystack {
		if v == needle[i] {
			i++
			if i == len(needle) {
				return true
			}
		}
	}
	return false
}

func TestDockerGenerator_Subscribe_ZeroTimestampDoesNotRewindSince(t *testing.T) {
	var connects int32
	var mu sync.Mutex
	var secondSince string

	mock := newMockDockerClient()
	mock.eventsFunc = func(ctx context.Context, options events.ListOptions) (<-chan events.Message, <-chan error) {
		n := atomic.AddInt32(&connects, 1)
		errCh := make(chan error)
		if n == 1 {
			eventCh := make(chan events.Message, 1)
			// A zero-timestamp event must NOT rewind `since` to the epoch.
			eventCh <- events.Message{
				ID: "c", Status: "start", TimeNano: 0,
				Actor: events.Actor{Attributes: map[string]string{"name": "app"}},
			}
			close(eventCh) // -> reconnect
			return eventCh, errCh
		}
		mu.Lock()
		secondSince = options.Since
		mu.Unlock()
		return make(chan events.Message), errCh
	}

	gen := fastGenerator(mock)
	ctx, cancel := context.WithCancel(context.Background())
	ch, err := gen.Subscribe(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for i := 0; i < 200; i++ {
		if atomic.LoadInt32(&connects) >= 2 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	cancel()
	for range ch {
	}

	mu.Lock()
	defer mu.Unlock()
	if secondSince == "" {
		t.Fatal("expected a Since value on the second connection")
	}
	if strings.HasPrefix(secondSince, "1970") {
		t.Errorf("expected Since not to be rewound to the epoch, got %q", secondSince)
	}
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
