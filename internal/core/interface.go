package core

import (
	"context"

	"github.com/auto-dns/docker-coredns-sync/internal/domain"
	"github.com/auto-dns/docker-coredns-sync/internal/intent"
)

type generator interface {
	Subscribe(ctx context.Context) (<-chan domain.ContainerEvent, error)
}

type upstreamRegistry interface {
	LockTransaction(ctx context.Context, key []string, fn func() error) error
	List(ctx context.Context) ([]*intent.RecordIntent, error)
	Register(ctx context.Context, record *intent.RecordIntent) error
	Remove(ctx context.Context, record *intent.RecordIntent) error
	Close() error
}
