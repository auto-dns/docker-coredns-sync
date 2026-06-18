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

// resyncPruneThreshold is the number of consecutive resyncs a running container
// must be absent from the live set before it is pruned. Debouncing avoids
// removing a container that is only transiently missing from a single snapshot
// (e.g. mid-restart, or a list race).
const resyncPruneThreshold = 2

// RetainRunning reconciles tracked state against the set of currently-running
// container IDs reported on a (re)connection to the Docker daemon, used to
// catch stop/die events that may have been missed (e.g. the daemon restarted
// and lost its event history). A running container absent from the set has its
// miss counter incremented and is marked removed only once it has been absent
// for resyncPruneThreshold consecutive resyncs; a container present in the set
// has its counter reset. It returns the number of containers newly marked
// removed.
func (s *MemoryState) RetainRunning(runningIds map[string]struct{}) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	removed := 0
	for id, cs := range s.containers {
		if cs.Status != domain.StatusRunning {
			continue
		}
		if _, ok := runningIds[id]; ok {
			cs.missedResyncs = 0
			continue
		}
		cs.missedResyncs++
		if cs.missedResyncs >= resyncPruneThreshold {
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
