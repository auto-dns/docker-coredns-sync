package core

import (
	"sync"
	"time"

	"github.com/auto-dns/docker-coredns-sync/internal/intent"
)

// ContainerState holds state derived from container events.
type ContainerState struct {
	ContainerID   string
	ContainerName string
	Created       time.Time
	LastUpdated   time.Time
	RecordIntents []*intent.RecordIntent
	Status        string // "running", "removed"
}

// StateTracker stores container state safely.
type StateTracker struct {
	mu         sync.RWMutex
	containers map[string]*ContainerState
}

// NewStateTracker creates a new tracker.
func NewStateTracker() *StateTracker {
	return &StateTracker{
		containers: make(map[string]*ContainerState),
	}
}

// Upsert inserts or updates the state for a container.
func (s *StateTracker) Upsert(containerID, containerName string, created time.Time, intents []*intent.RecordIntent, status string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.containers[containerID] = &ContainerState{
		ContainerID:   containerID,
		ContainerName: containerName,
		Created:       created,
		RecordIntents: intents,
		Status:        status,
		LastUpdated:   time.Now(),
	}
}

// MarkRemoved marks a container state as removed.
func (s *StateTracker) MarkRemoved(containerID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if state, exists := s.containers[containerID]; exists {
		state.Status = "removed"
		state.LastUpdated = time.Now()
		return true
	}
	return false
}

// GetAllDesiredRecordIntents returns all record intents from running containers.
func (s *StateTracker) GetAllDesiredRecordIntents() []*intent.RecordIntent {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var intents []*intent.RecordIntent
	for _, cs := range s.containers {
		if cs.Status == "running" {
			intents = append(intents, cs.RecordIntents...)
		}
	}
	return intents
}
