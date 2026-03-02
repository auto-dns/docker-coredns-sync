package core

import (
	"context"
	"sync"
	"time"

	"github.com/auto-dns/docker-coredns-sync/internal/domain"
)

type mockGenerator struct {
	mu            sync.Mutex
	subscribeFunc func(ctx context.Context) (<-chan domain.ContainerEvent, error)
	subscribeCalled bool
}

func (m *mockGenerator) Subscribe(ctx context.Context) (<-chan domain.ContainerEvent, error) {
	m.mu.Lock()
	m.subscribeCalled = true
	m.mu.Unlock()

	if m.subscribeFunc != nil {
		return m.subscribeFunc(ctx)
	}
	ch := make(chan domain.ContainerEvent)
	close(ch)
	return ch, nil
}

type mockState struct {
	mu                      sync.Mutex
	upsertFunc              func(containerId, containerName string, created time.Time, intents []*domain.RecordIntent, status string)
	markRemovedFunc         func(containerId string) bool
	getAllDesiredFunc       func() []*domain.RecordIntent
	
	upsertCalled            bool
	markRemovedCalled       bool
	getAllDesiredCalled     bool
	
	lastUpsertContainerId   string
	lastUpsertContainerName string
	lastUpsertIntents       []*domain.RecordIntent
	lastMarkRemovedId       string
}

func (m *mockState) Upsert(containerId, containerName string, created time.Time, intents []*domain.RecordIntent, status string) {
	m.mu.Lock()
	m.upsertCalled = true
	m.lastUpsertContainerId = containerId
	m.lastUpsertContainerName = containerName
	m.lastUpsertIntents = intents
	m.mu.Unlock()

	if m.upsertFunc != nil {
		m.upsertFunc(containerId, containerName, created, intents, status)
	}
}

func (m *mockState) MarkRemoved(containerId string) bool {
	m.mu.Lock()
	m.markRemovedCalled = true
	m.lastMarkRemovedId = containerId
	m.mu.Unlock()

	if m.markRemovedFunc != nil {
		return m.markRemovedFunc(containerId)
	}
	return true
}

func (m *mockState) GetAllDesiredRecordIntents() []*domain.RecordIntent {
	m.mu.Lock()
	m.getAllDesiredCalled = true
	m.mu.Unlock()

	if m.getAllDesiredFunc != nil {
		return m.getAllDesiredFunc()
	}
	return []*domain.RecordIntent{}
}

func (m *mockState) WasUpsertCalled() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.upsertCalled
}

func (m *mockState) WasMarkRemovedCalled() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.markRemovedCalled
}

type mockRegistry struct {
	mu                  sync.Mutex
	lockTransactionFunc func(ctx context.Context, keys []string, fn func() error) error
	listFunc            func(ctx context.Context) ([]*domain.RecordIntent, error)
	registerFunc        func(ctx context.Context, record *domain.RecordIntent) error
	removeFunc          func(ctx context.Context, record *domain.RecordIntent) error
	closeFunc           func() error
	
	lockTransactionCalled bool
	listCalled            bool
	registerCalled        bool
	removeCalled          bool
	closeCalled           bool
	
	registeredRecords     []*domain.RecordIntent
	removedRecords        []*domain.RecordIntent
}

func (m *mockRegistry) LockTransaction(ctx context.Context, keys []string, fn func() error) error {
	m.mu.Lock()
	m.lockTransactionCalled = true
	m.mu.Unlock()

	if m.lockTransactionFunc != nil {
		return m.lockTransactionFunc(ctx, keys, fn)
	}
	return fn()
}

func (m *mockRegistry) List(ctx context.Context) ([]*domain.RecordIntent, error) {
	m.mu.Lock()
	m.listCalled = true
	m.mu.Unlock()

	if m.listFunc != nil {
		return m.listFunc(ctx)
	}
	return []*domain.RecordIntent{}, nil
}

func (m *mockRegistry) Register(ctx context.Context, record *domain.RecordIntent) error {
	m.mu.Lock()
	m.registerCalled = true
	m.registeredRecords = append(m.registeredRecords, record)
	m.mu.Unlock()

	if m.registerFunc != nil {
		return m.registerFunc(ctx, record)
	}
	return nil
}

func (m *mockRegistry) Remove(ctx context.Context, record *domain.RecordIntent) error {
	m.mu.Lock()
	m.removeCalled = true
	m.removedRecords = append(m.removedRecords, record)
	m.mu.Unlock()

	if m.removeFunc != nil {
		return m.removeFunc(ctx, record)
	}
	return nil
}

func (m *mockRegistry) Close() error {
	m.mu.Lock()
	m.closeCalled = true
	m.mu.Unlock()

	if m.closeFunc != nil {
		return m.closeFunc()
	}
	return nil
}

func (m *mockRegistry) WasRegisterCalled() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.registerCalled
}

func (m *mockRegistry) WasRemoveCalled() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.removeCalled
}

func (m *mockRegistry) WasListCalled() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.listCalled
}

func (m *mockRegistry) WasLockTransactionCalled() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.lockTransactionCalled
}

func (m *mockRegistry) WasCloseCalled() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.closeCalled
}

func (m *mockRegistry) GetRegisteredRecords() []*domain.RecordIntent {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.registeredRecords
}

func (m *mockRegistry) GetRemovedRecords() []*domain.RecordIntent {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.removedRecords
}
