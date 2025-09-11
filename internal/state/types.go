package state

import (
	"time"

	"github.com/auto-dns/docker-coredns-sync/internal/domain"
)

type containerState struct {
	ContainerID   string
	ContainerName string
	Created       time.Time
	LastUpdated   time.Time
	RecordIntents []*domain.RecordIntent
	Status        string // "running", "removed"
}
