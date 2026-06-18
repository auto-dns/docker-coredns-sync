package core

import (
	"context"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/auto-dns/docker-coredns-sync/internal/config"
	"github.com/auto-dns/docker-coredns-sync/internal/domain"
	"github.com/rs/zerolog"
)

func engineTestLogger() zerolog.Logger {
	return zerolog.New(io.Discard)
}

func testAppConfig() *config.AppConfig {
	return &config.AppConfig{
		DockerLabelPrefix: "coredns",
		HostIPv4:          "192.168.1.1",
		HostIPv6:          "",
		Hostname:          "test-host",
		PollInterval:      1,
	}
}

func TestNewSyncEngine(t *testing.T) {
	gen := &mockGenerator{}
	state := &mockState{}
	reg := &mockRegistry{}
	cfg := testAppConfig()
	logger := testLogger()

	engine := NewSyncEngine(logger, cfg, gen, reg, state)

	if engine == nil {
		t.Fatal("expected non-nil engine")
	}
	if engine.gen != gen {
		t.Error("expected generator to be set")
	}
	if engine.state != state {
		t.Error("expected state to be set")
	}
	if engine.reg != reg {
		t.Error("expected registry to be set")
	}
}

func TestSyncEngine_handleEvent_EmptyContainerId(t *testing.T) {
	gen := &mockGenerator{}
	state := &mockState{}
	reg := &mockRegistry{}
	cfg := testAppConfig()

	engine := NewSyncEngine(engineTestLogger(), cfg, gen, reg, state)

	event := domain.ContainerEvent{
		Container: domain.Container{
			Id:   "", // empty
			Name: "test",
		},
		EventType: domain.EventTypeContainerStarted,
	}

	engine.handleEvent(event)

	// Should not update state for empty container ID
	if state.upsertCalled {
		t.Error("expected Upsert not to be called for empty container ID")
	}
}

func TestSyncEngine_handleEvent_InvalidEventType(t *testing.T) {
	gen := &mockGenerator{}
	state := &mockState{}
	reg := &mockRegistry{}
	cfg := testAppConfig()

	engine := NewSyncEngine(engineTestLogger(), cfg, gen, reg, state)

	event := domain.ContainerEvent{
		Container: domain.Container{
			Id:   "container-123",
			Name: "test",
		},
		EventType: domain.EventType("invalid_event"),
	}

	engine.handleEvent(event)

	// Should not update state for invalid event type
	if state.upsertCalled {
		t.Error("expected Upsert not to be called for invalid event type")
	}
}

func TestSyncEngine_handleEvent_Resync(t *testing.T) {
	gen := &mockGenerator{}
	var gotIds map[string]struct{}
	state := &mockState{
		retainRunningFunc: func(runningIds map[string]struct{}) int {
			gotIds = runningIds
			return 1
		},
	}
	reg := &mockRegistry{}
	cfg := testAppConfig()

	engine := NewSyncEngine(engineTestLogger(), cfg, gen, reg, state)

	engine.handleEvent(domain.ContainerEvent{
		EventType:           domain.EventTypeResync,
		RunningContainerIds: []string{"a", "b"},
	})

	if !state.retainRunningCalled {
		t.Fatal("expected RetainRunning to be called on resync event")
	}
	if _, ok := gotIds["a"]; !ok {
		t.Error("expected running id 'a' in set")
	}
	if _, ok := gotIds["b"]; !ok {
		t.Error("expected running id 'b' in set")
	}
	if state.upsertCalled || state.markRemovedCalled {
		t.Error("resync event should not Upsert or MarkRemoved directly")
	}
}

func TestSyncEngine_handleEvent_StartEvent(t *testing.T) {
	gen := &mockGenerator{}
	state := &mockState{}
	reg := &mockRegistry{}
	cfg := testAppConfig()

	engine := NewSyncEngine(engineTestLogger(), cfg, gen, reg, state)

	event := domain.ContainerEvent{
		Container: domain.Container{
			Id:      "container-123",
			Name:    "my-app",
			Created: time.Now(),
			Labels: map[string]string{
				"coredns.enabled":     "true",
				"coredns.a.web.name":  "app.example.com",
				"coredns.a.web.value": "192.168.1.100",
			},
		},
		EventType: domain.EventTypeContainerStarted,
	}

	engine.handleEvent(event)

	if !state.upsertCalled {
		t.Error("expected Upsert to be called for start event")
	}
	if state.lastUpsertContainerId != "container-123" {
		t.Errorf("expected container ID 'container-123', got %q", state.lastUpsertContainerId)
	}
}

func TestSyncEngine_handleEvent_InitialDetection(t *testing.T) {
	gen := &mockGenerator{}
	state := &mockState{}
	reg := &mockRegistry{}
	cfg := testAppConfig()

	engine := NewSyncEngine(engineTestLogger(), cfg, gen, reg, state)

	event := domain.ContainerEvent{
		Container: domain.Container{
			Id:      "container-123",
			Name:    "my-app",
			Created: time.Now(),
			Labels: map[string]string{
				"coredns.enabled":     "true",
				"coredns.a.web.name":  "app.example.com",
				"coredns.a.web.value": "192.168.1.100",
			},
		},
		EventType: domain.EventTypeInitialContainerDetection,
	}

	engine.handleEvent(event)

	if !state.upsertCalled {
		t.Error("expected Upsert to be called for initial detection")
	}
}

func TestSyncEngine_handleEvent_DieEvent(t *testing.T) {
	gen := &mockGenerator{}
	state := &mockState{}
	reg := &mockRegistry{}
	cfg := testAppConfig()

	engine := NewSyncEngine(engineTestLogger(), cfg, gen, reg, state)

	event := domain.ContainerEvent{
		Container: domain.Container{
			Id:   "container-123",
			Name: "my-app",
		},
		EventType: domain.EventTypeContainerDied,
	}

	engine.handleEvent(event)

	if !state.markRemovedCalled {
		t.Error("expected MarkRemoved to be called for die event")
	}
	if state.lastMarkRemovedId != "container-123" {
		t.Errorf("expected container ID 'container-123', got %q", state.lastMarkRemovedId)
	}
}

func TestSyncEngine_handleEvent_StopEvent(t *testing.T) {
	gen := &mockGenerator{}
	state := &mockState{}
	reg := &mockRegistry{}
	cfg := testAppConfig()

	engine := NewSyncEngine(engineTestLogger(), cfg, gen, reg, state)

	event := domain.ContainerEvent{
		Container: domain.Container{
			Id:   "container-123",
			Name: "my-app",
		},
		EventType: domain.EventTypeContainerStopped,
	}

	engine.handleEvent(event)

	if !state.markRemovedCalled {
		t.Error("expected MarkRemoved to be called for stop event")
	}
}

func TestSyncEngine_handleEvent_StartWithNoIntents(t *testing.T) {
	gen := &mockGenerator{}
	state := &mockState{}
	reg := &mockRegistry{}
	cfg := testAppConfig()

	engine := NewSyncEngine(engineTestLogger(), cfg, gen, reg, state)

	// Container without coredns labels - no intents generated
	event := domain.ContainerEvent{
		Container: domain.Container{
			Id:      "container-123",
			Name:    "my-app",
			Created: time.Now(),
			Labels: map[string]string{
				"some.other.label": "value",
			},
		},
		EventType: domain.EventTypeContainerStarted,
	}

	engine.handleEvent(event)

	// Should not call Upsert if no intents are generated
	if state.upsertCalled {
		t.Error("expected Upsert not to be called when no intents generated")
	}
}

func TestSyncEngine_Run_SubscribeError(t *testing.T) {
	gen := &mockGenerator{
		subscribeFunc: func(ctx context.Context) (<-chan domain.ContainerEvent, error) {
			return nil, errors.New("subscribe failed")
		},
	}
	state := &mockState{}
	reg := &mockRegistry{}
	cfg := testAppConfig()

	engine := NewSyncEngine(engineTestLogger(), cfg, gen, reg, state)

	ctx := context.Background()
	err := engine.Run(ctx)

	if err == nil {
		t.Error("expected error from Run when Subscribe fails")
	}
	if !gen.subscribeCalled {
		t.Error("expected Subscribe to be called")
	}
}

func TestSyncEngine_Run_ContextCancellation(t *testing.T) {
	eventCh := make(chan domain.ContainerEvent)

	gen := &mockGenerator{
		subscribeFunc: func(ctx context.Context) (<-chan domain.ContainerEvent, error) {
			return eventCh, nil
		},
	}
	state := &mockState{
		getAllDesiredFunc: func() []*domain.RecordIntent {
			return []*domain.RecordIntent{}
		},
	}
	reg := &mockRegistry{}
	cfg := testAppConfig()
	cfg.PollInterval = 10 // Long enough that we cancel before it fires

	engine := NewSyncEngine(engineTestLogger(), cfg, gen, reg, state)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := engine.Run(ctx)

	if err != context.DeadlineExceeded {
		t.Errorf("expected context.DeadlineExceeded, got %v", err)
	}
	if !reg.WasCloseCalled() {
		t.Error("expected Close to be called on shutdown")
	}
}

func TestSyncEngine_Run_ProcessesEvents(t *testing.T) {
	eventCh := make(chan domain.ContainerEvent, 10)

	gen := &mockGenerator{
		subscribeFunc: func(ctx context.Context) (<-chan domain.ContainerEvent, error) {
			return eventCh, nil
		},
	}
	state := &mockState{
		getAllDesiredFunc: func() []*domain.RecordIntent {
			return []*domain.RecordIntent{}
		},
	}
	reg := &mockRegistry{}
	cfg := testAppConfig()
	cfg.PollInterval = 10

	engine := NewSyncEngine(engineTestLogger(), cfg, gen, reg, state)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	// Send an event
	go func() {
		eventCh <- domain.ContainerEvent{
			Container: domain.Container{
				Id:      "container-123",
				Name:    "my-app",
				Created: time.Now(),
				Labels: map[string]string{
					"coredns.enabled":     "true",
					"coredns.a.web.name":  "app.example.com",
					"coredns.a.web.value": "192.168.1.100",
				},
			},
			EventType: domain.EventTypeContainerStarted,
		}
	}()

	go func() {
		time.Sleep(200 * time.Millisecond)
		cancel()
	}()

	engine.Run(ctx)

	// Wait a bit for event processing
	time.Sleep(50 * time.Millisecond)

	if !state.WasUpsertCalled() {
		t.Error("expected Upsert to be called after processing event")
	}
}

func TestSyncEngine_Run_ReconciliationLoop(t *testing.T) {
	eventCh := make(chan domain.ContainerEvent)
	close(eventCh)

	gen := &mockGenerator{
		subscribeFunc: func(ctx context.Context) (<-chan domain.ContainerEvent, error) {
			return eventCh, nil
		},
	}

	rec, _ := domain.NewA("app.example.com", "192.168.1.1")
	desiredIntents := []*domain.RecordIntent{
		{
			ContainerId:   "container-123",
			ContainerName: "my-app",
			Created:       time.Now(),
			Hostname:      "test-host",
			Record:        rec,
		},
	}

	state := &mockState{
		getAllDesiredFunc: func() []*domain.RecordIntent {
			return desiredIntents
		},
	}
	reg := &mockRegistry{
		listFunc: func(ctx context.Context) ([]*domain.RecordIntent, error) {
			return []*domain.RecordIntent{}, nil
		},
	}
	cfg := testAppConfig()
	cfg.PollInterval = 1 // 1 second

	engine := NewSyncEngine(engineTestLogger(), cfg, gen, reg, state)

	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()

	go func() {
		engine.Run(ctx)
	}()

	// Wait for at least one reconciliation tick
	time.Sleep(1200 * time.Millisecond)
	cancel()

	if !reg.WasLockTransactionCalled() {
		t.Error("expected LockTransaction to be called during reconciliation")
	}
	if !reg.WasListCalled() {
		t.Error("expected List to be called during reconciliation")
	}
	if !state.getAllDesiredCalled {
		t.Error("expected GetAllDesiredRecordIntents to be called during reconciliation")
	}
}

func TestSyncEngine_Run_RegistersNewRecords(t *testing.T) {
	eventCh := make(chan domain.ContainerEvent)
	close(eventCh)

	gen := &mockGenerator{
		subscribeFunc: func(ctx context.Context) (<-chan domain.ContainerEvent, error) {
			return eventCh, nil
		},
	}

	rec, _ := domain.NewA("app.example.com", "192.168.1.1")
	desiredIntents := []*domain.RecordIntent{
		{
			ContainerId:   "container-123",
			ContainerName: "my-app",
			Created:       time.Now(),
			Hostname:      "test-host",
			Record:        rec,
		},
	}

	state := &mockState{
		getAllDesiredFunc: func() []*domain.RecordIntent {
			return desiredIntents
		},
	}
	reg := &mockRegistry{
		listFunc: func(ctx context.Context) ([]*domain.RecordIntent, error) {
			return []*domain.RecordIntent{}, nil // No existing records
		},
	}
	cfg := testAppConfig()
	cfg.PollInterval = 1

	engine := NewSyncEngine(engineTestLogger(), cfg, gen, reg, state)

	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()

	go func() {
		engine.Run(ctx)
	}()

	time.Sleep(1200 * time.Millisecond)
	cancel()

	if !reg.WasRegisterCalled() {
		t.Error("expected Register to be called for new record")
	}
	if len(reg.GetRegisteredRecords()) != 1 {
		t.Errorf("expected 1 registered record, got %d", len(reg.GetRegisteredRecords()))
	}
}

func TestSyncEngine_Run_RemovesStaleRecords(t *testing.T) {
	eventCh := make(chan domain.ContainerEvent)
	close(eventCh)

	gen := &mockGenerator{
		subscribeFunc: func(ctx context.Context) (<-chan domain.ContainerEvent, error) {
			return eventCh, nil
		},
	}

	state := &mockState{
		getAllDesiredFunc: func() []*domain.RecordIntent {
			return []*domain.RecordIntent{} // No desired records
		},
	}

	rec, _ := domain.NewA("stale.example.com", "192.168.1.99")
	staleRecord := &domain.RecordIntent{
		ContainerId:   "old-container",
		ContainerName: "old-app",
		Created:       time.Now().Add(-time.Hour),
		Hostname:      "test-host", // Owned by this host
		Record:        rec,
	}

	reg := &mockRegistry{
		listFunc: func(ctx context.Context) ([]*domain.RecordIntent, error) {
			return []*domain.RecordIntent{staleRecord}, nil
		},
	}
	cfg := testAppConfig()
	cfg.PollInterval = 1

	engine := NewSyncEngine(engineTestLogger(), cfg, gen, reg, state)

	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()

	go func() {
		engine.Run(ctx)
	}()

	time.Sleep(1200 * time.Millisecond)
	cancel()

	if !reg.WasRemoveCalled() {
		t.Error("expected Remove to be called for stale record")
	}
	if len(reg.GetRemovedRecords()) != 1 {
		t.Errorf("expected 1 removed record, got %d", len(reg.GetRemovedRecords()))
	}
}

func TestSyncEngine_Run_ListError(t *testing.T) {
	eventCh := make(chan domain.ContainerEvent)
	close(eventCh)

	gen := &mockGenerator{
		subscribeFunc: func(ctx context.Context) (<-chan domain.ContainerEvent, error) {
			return eventCh, nil
		},
	}

	state := &mockState{
		getAllDesiredFunc: func() []*domain.RecordIntent {
			return []*domain.RecordIntent{}
		},
	}
	reg := &mockRegistry{
		listFunc: func(ctx context.Context) ([]*domain.RecordIntent, error) {
			return nil, errors.New("etcd unavailable")
		},
	}
	cfg := testAppConfig()
	cfg.PollInterval = 1

	engine := NewSyncEngine(engineTestLogger(), cfg, gen, reg, state)

	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()

	go func() {
		engine.Run(ctx)
	}()

	time.Sleep(1200 * time.Millisecond)
	cancel()

	if !reg.WasListCalled() {
		t.Error("expected List to be called")
	}
	// Should not crash, just log error
}

func TestSyncEngine_Run_RemoveError(t *testing.T) {
	eventCh := make(chan domain.ContainerEvent)
	close(eventCh)

	gen := &mockGenerator{
		subscribeFunc: func(ctx context.Context) (<-chan domain.ContainerEvent, error) {
			return eventCh, nil
		},
	}

	state := &mockState{
		getAllDesiredFunc: func() []*domain.RecordIntent {
			return []*domain.RecordIntent{}
		},
	}

	rec, _ := domain.NewA("stale.example.com", "192.168.1.99")
	staleRecord := &domain.RecordIntent{
		ContainerId:   "old-container",
		ContainerName: "old-app",
		Created:       time.Now().Add(-time.Hour),
		Hostname:      "test-host",
		Record:        rec,
	}

	reg := &mockRegistry{
		listFunc: func(ctx context.Context) ([]*domain.RecordIntent, error) {
			return []*domain.RecordIntent{staleRecord}, nil
		},
		removeFunc: func(ctx context.Context, ri *domain.RecordIntent) error {
			return errors.New("remove failed")
		},
	}
	cfg := testAppConfig()
	cfg.PollInterval = 1

	engine := NewSyncEngine(engineTestLogger(), cfg, gen, reg, state)

	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()

	go func() {
		engine.Run(ctx)
	}()

	time.Sleep(1200 * time.Millisecond)
	cancel()

	if !reg.WasRemoveCalled() {
		t.Error("expected Remove to be called")
	}
	// Should not crash, just log error
}

func TestSyncEngine_Run_RegisterError(t *testing.T) {
	eventCh := make(chan domain.ContainerEvent)
	close(eventCh)

	gen := &mockGenerator{
		subscribeFunc: func(ctx context.Context) (<-chan domain.ContainerEvent, error) {
			return eventCh, nil
		},
	}

	rec, _ := domain.NewA("app.example.com", "192.168.1.1")
	desiredIntents := []*domain.RecordIntent{
		{
			ContainerId:   "container-123",
			ContainerName: "my-app",
			Created:       time.Now(),
			Hostname:      "test-host",
			Record:        rec,
		},
	}

	state := &mockState{
		getAllDesiredFunc: func() []*domain.RecordIntent {
			return desiredIntents
		},
	}
	reg := &mockRegistry{
		listFunc: func(ctx context.Context) ([]*domain.RecordIntent, error) {
			return []*domain.RecordIntent{}, nil
		},
		registerFunc: func(ctx context.Context, ri *domain.RecordIntent) error {
			return errors.New("register failed")
		},
	}
	cfg := testAppConfig()
	cfg.PollInterval = 1

	engine := NewSyncEngine(engineTestLogger(), cfg, gen, reg, state)

	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()

	go func() {
		engine.Run(ctx)
	}()

	time.Sleep(1200 * time.Millisecond)
	cancel()

	if !reg.WasRegisterCalled() {
		t.Error("expected Register to be called")
	}
	// Should not crash, just log error
}

func TestSyncEngine_Run_CloseError(t *testing.T) {
	eventCh := make(chan domain.ContainerEvent)
	close(eventCh)

	gen := &mockGenerator{
		subscribeFunc: func(ctx context.Context) (<-chan domain.ContainerEvent, error) {
			return eventCh, nil
		},
	}

	state := &mockState{
		getAllDesiredFunc: func() []*domain.RecordIntent {
			return []*domain.RecordIntent{}
		},
	}
	reg := &mockRegistry{
		closeFunc: func() error {
			return errors.New("close failed")
		},
	}
	cfg := testAppConfig()
	cfg.PollInterval = 10 // Long enough to not trigger

	engine := NewSyncEngine(engineTestLogger(), cfg, gen, reg, state)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := engine.Run(ctx)

	if !reg.WasCloseCalled() {
		t.Error("expected Close to be called")
	}
	// Context error should be returned, not Close error
	if err != context.DeadlineExceeded {
		t.Errorf("expected context.DeadlineExceeded, got %v", err)
	}
}

type recordingReporter struct {
	mu    sync.Mutex
	calls []error
}

func (r *recordingReporter) RecordReconcile(err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, err)
}

func (r *recordingReporter) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.calls)
}

func TestSyncEngine_Run_DryRunSkipsWrites(t *testing.T) {
	eventCh := make(chan domain.ContainerEvent)
	close(eventCh)

	gen := &mockGenerator{
		subscribeFunc: func(ctx context.Context) (<-chan domain.ContainerEvent, error) {
			return eventCh, nil
		},
	}

	rec, _ := domain.NewA("app.example.com", "192.168.1.1")
	desiredIntents := []*domain.RecordIntent{
		{
			ContainerId:   "container-123",
			ContainerName: "my-app",
			Created:       time.Now(),
			Hostname:      "test-host",
			Record:        rec,
		},
	}

	staleRec, _ := domain.NewA("stale.example.com", "192.168.1.99")
	staleRecord := &domain.RecordIntent{
		ContainerId:   "old-container",
		ContainerName: "old-app",
		Created:       time.Now().Add(-time.Hour),
		Hostname:      "test-host",
		Record:        staleRec,
	}

	state := &mockState{
		getAllDesiredFunc: func() []*domain.RecordIntent {
			return desiredIntents
		},
	}
	reg := &mockRegistry{
		listFunc: func(ctx context.Context) ([]*domain.RecordIntent, error) {
			return []*domain.RecordIntent{staleRecord}, nil
		},
	}
	cfg := testAppConfig()
	cfg.PollInterval = 1
	cfg.DryRun = true

	engine := NewSyncEngine(engineTestLogger(), cfg, gen, reg, state)

	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()

	go func() {
		engine.Run(ctx)
	}()

	time.Sleep(1200 * time.Millisecond)
	cancel()

	// Reconciliation still runs (list + plan), but nothing is written.
	if !reg.WasListCalled() {
		t.Error("expected List to be called even in dry-run")
	}
	if reg.WasRegisterCalled() {
		t.Error("expected Register NOT to be called in dry-run")
	}
	if reg.WasRemoveCalled() {
		t.Error("expected Remove NOT to be called in dry-run")
	}
}

func TestSyncEngine_Run_ReportsReconcileResult(t *testing.T) {
	eventCh := make(chan domain.ContainerEvent)
	close(eventCh)

	gen := &mockGenerator{
		subscribeFunc: func(ctx context.Context) (<-chan domain.ContainerEvent, error) {
			return eventCh, nil
		},
	}
	state := &mockState{
		getAllDesiredFunc: func() []*domain.RecordIntent {
			return []*domain.RecordIntent{}
		},
	}
	reg := &mockRegistry{
		listFunc: func(ctx context.Context) ([]*domain.RecordIntent, error) {
			return []*domain.RecordIntent{}, nil
		},
	}
	cfg := testAppConfig()
	cfg.PollInterval = 1

	reporter := &recordingReporter{}
	engine := NewSyncEngine(engineTestLogger(), cfg, gen, reg, state)
	engine.SetReconcileReporter(reporter)

	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()

	go func() {
		engine.Run(ctx)
	}()

	time.Sleep(1200 * time.Millisecond)
	cancel()

	if reporter.count() == 0 {
		t.Error("expected reconcile reporter to be notified at least once")
	}
}

func TestSyncEngine_Run_EventChannelClosed(t *testing.T) {
	eventCh := make(chan domain.ContainerEvent)

	gen := &mockGenerator{
		subscribeFunc: func(ctx context.Context) (<-chan domain.ContainerEvent, error) {
			return eventCh, nil
		},
	}

	state := &mockState{
		getAllDesiredFunc: func() []*domain.RecordIntent {
			return []*domain.RecordIntent{}
		},
	}
	reg := &mockRegistry{}
	cfg := testAppConfig()
	cfg.PollInterval = 10

	engine := NewSyncEngine(engineTestLogger(), cfg, gen, reg, state)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	// Close the event channel after a short delay
	go func() {
		time.Sleep(100 * time.Millisecond)
		close(eventCh)
	}()

	engine.Run(ctx)

	// Should continue running even after event channel closes
	// until context is cancelled
}
