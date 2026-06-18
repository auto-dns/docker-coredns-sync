package state

import (
	"time"

	"github.com/auto-dns/docker-coredns-sync/internal/domain"
)

type containerState struct {
	ContainerId   string
	ContainerName string
	Created       time.Time
	LastUpdated   time.Time
	RecordIntents []*domain.RecordIntent
	Status        domain.ContainerStatus
	// missedResyncs counts consecutive resyncs in which this running container
	// was absent from the live set. Pruning is debounced on this count so a
	// container that is only transiently missing (e.g. mid-restart at the
	// instant of a single snapshot) is not removed.
	missedResyncs int
}
