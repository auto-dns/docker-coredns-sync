package registry

import (
	"context"

	"github.com/auto-dns/docker-coredns-sync/internal/intent"
)

type Registry interface {
	LockTransaction(ctx context.Context, key []string, fn func() error) error
	List(ctx context.Context) ([]*intent.RecordIntent, error)
	Register(ctx context.Context, record *intent.RecordIntent) error
	Remove(ctx context.Context, record *intent.RecordIntent) error
	Close() error
}
