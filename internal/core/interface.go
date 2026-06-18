package core

import (
	"context"
	"time"

	"github.com/auto-dns/docker-coredns-sync/internal/domain"
)

type generator interface {
	Subscribe(ctx context.Context) (<-chan domain.ContainerEvent, error)
}

type state interface {
	Upsert(containerId, containerName string, created time.Time, intents []*domain.RecordIntent, status domain.ContainerStatus)
	MarkRemoved(containerId string) bool
	RetainRunning(runningIds map[string]struct{}) int
	GetAllDesiredRecordIntents() []*domain.RecordIntent
}

type upstreamRegistry interface {
	LockTransaction(ctx context.Context, key []string, fn func() error) error
	List(ctx context.Context) ([]*domain.RecordIntent, error)
	Register(ctx context.Context, record *domain.RecordIntent) error
	Remove(ctx context.Context, record *domain.RecordIntent) error
	Close() error
}

// reconcileReporter is an optional observer of reconciliation outcomes, used to
// feed liveness/readiness reporting. A nil error indicates a successful pass.
type reconcileReporter interface {
	RecordReconcile(err error)
}

// reconcileMetrics is an optional sink for quantitative reconciliation metrics.
// added/removed are the records actually applied this pass (zero in dry-run),
// skipped is the number of desired records dropped during conflict filtering.
type reconcileMetrics interface {
	ObserveReconcile(duration time.Duration, added, removed, skipped int, err error)
}
