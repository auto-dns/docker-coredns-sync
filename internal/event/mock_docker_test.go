package event

import (
	"context"
	"sync"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/events"
)

type mockDockerClient struct {
	mu sync.Mutex

	containerListFunc func(ctx context.Context, options container.ListOptions) ([]container.Summary, error)
	eventsFunc        func(ctx context.Context, options events.ListOptions) (<-chan events.Message, <-chan error)
	closeFunc         func() error

	containerListCalled bool
	eventsCalled        bool
	closeCalled         bool
}

func newMockDockerClient() *mockDockerClient {
	return &mockDockerClient{}
}

func (m *mockDockerClient) ContainerList(ctx context.Context, options container.ListOptions) ([]container.Summary, error) {
	m.mu.Lock()
	m.containerListCalled = true
	m.mu.Unlock()

	if m.containerListFunc != nil {
		return m.containerListFunc(ctx, options)
	}
	return []container.Summary{}, nil
}

func (m *mockDockerClient) Events(ctx context.Context, options events.ListOptions) (<-chan events.Message, <-chan error) {
	m.mu.Lock()
	m.eventsCalled = true
	m.mu.Unlock()

	if m.eventsFunc != nil {
		return m.eventsFunc(ctx, options)
	}
	msgCh := make(chan events.Message)
	errCh := make(chan error)
	close(msgCh)
	return msgCh, errCh
}

func (m *mockDockerClient) Close() error {
	m.mu.Lock()
	m.closeCalled = true
	m.mu.Unlock()

	if m.closeFunc != nil {
		return m.closeFunc()
	}
	return nil
}
