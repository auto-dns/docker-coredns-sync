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
	Upsert(containerID, containerName string, created time.Time, intents []*domain.RecordIntent, status string)
	MarkRemoved(containerID string) bool
	GetAllDesiredRecordIntents() []*domain.RecordIntent
}

type upstreamRegistry interface {
	LockTransaction(ctx context.Context, key []string, fn func() error) error
	List(ctx context.Context) ([]*domain.RecordIntent, error)
	Register(ctx context.Context, record *domain.RecordIntent) error
	Remove(ctx context.Context, record *domain.RecordIntent) error
	Close() error
}
