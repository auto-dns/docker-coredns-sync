package state

import (
	"sync"
	"time"

	"github.com/auto-dns/docker-coredns-sync/internal/domain"
)

// MemoryState stores container state safely.
type MemoryState struct {
	mu         sync.RWMutex
	containers map[string]*containerState
}

// NewStateTracker creates a new tracker.
func NewMemoryState() *MemoryState {
	return &MemoryState{
		containers: make(map[string]*containerState),
	}
}

// Upsert inserts or updates the state for a container.
func (s *MemoryState) Upsert(containerId, containerName string, created time.Time, intents []*domain.RecordIntent, status string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.containers[containerId] = &containerState{
		ContainerId:   containerId,
		ContainerName: containerName,
		Created:       created,
		RecordIntents: intents,
		Status:        status,
		LastUpdated:   time.Now(),
	}
}

// MarkRemoved marks a container state as removed.
func (s *MemoryState) MarkRemoved(containerId string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if state, exists := s.containers[containerId]; exists {
		state.Status = "removed"
		state.LastUpdated = time.Now()
		return true
	}
	return false
}

// GetAllDesiredRecordIntents returns all record intents from running containers.
func (s *MemoryState) GetAllDesiredRecordIntents() []*domain.RecordIntent {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var intents []*domain.RecordIntent
	for _, cs := range s.containers {
		if cs.Status == "running" {
			intents = append(intents, cs.RecordIntents...)
		}
	}
	return intents
}
