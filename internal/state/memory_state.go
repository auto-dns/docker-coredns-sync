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

// NewMemoryState creates a new in-memory state tracker.
func NewMemoryState() *MemoryState {
	return &MemoryState{
		containers: make(map[string]*containerState),
	}
}

// Upsert inserts or updates the state for a container.
//
// The intents slice is stored by reference and later handed to the
// reconciliation loop (via GetAllDesiredRecordIntents) without holding the
// lock. This is safe only because RecordIntents are treated as immutable once
// built: Upsert always replaces a container's entry wholesale rather than
// mutating an existing one in place. Do not mutate a stored RecordIntent.
func (s *MemoryState) Upsert(containerId, containerName string, created time.Time, intents []*domain.RecordIntent, status domain.ContainerStatus) {
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

// RetainRunning marks as removed any tracked container whose ID is not in the
// given set of currently-running container IDs. It is used to reconcile state
// after a (re)connection to the Docker daemon, when stop/die events may have
// been missed (e.g. the daemon restarted and lost its event history). It
// returns the number of containers newly marked removed.
func (s *MemoryState) RetainRunning(runningIds map[string]struct{}) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	removed := 0
	for id, cs := range s.containers {
		if cs.Status != domain.StatusRunning {
			continue
		}
		if _, ok := runningIds[id]; !ok {
			cs.Status = domain.StatusRemoved
			cs.LastUpdated = time.Now()
			removed++
		}
	}
	return removed
}

// MarkRemoved marks a container state as removed.
func (s *MemoryState) MarkRemoved(containerId string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if state, exists := s.containers[containerId]; exists {
		state.Status = domain.StatusRemoved
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
		if cs.Status == domain.StatusRunning {
			intents = append(intents, cs.RecordIntents...)
		}
	}
	return intents
}
