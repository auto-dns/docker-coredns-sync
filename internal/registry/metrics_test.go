package registry

import (
	"context"
	"errors"
	"sync"
	"testing"

	clientv3 "go.etcd.io/etcd/client/v3"
)

type countingMetrics struct {
	mu           sync.Mutex
	etcdErrors   int
	lockFailures int
}

func (c *countingMetrics) IncEtcdError() {
	c.mu.Lock()
	c.etcdErrors++
	c.mu.Unlock()
}

func (c *countingMetrics) IncLockFailure() {
	c.mu.Lock()
	c.lockFailures++
	c.mu.Unlock()
}

func (c *countingMetrics) snapshot() (etcdErrors, lockFailures int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.etcdErrors, c.lockFailures
}

func TestEtcdRegistry_Metrics_EtcdErrorOnList(t *testing.T) {
	mock := newMockEtcdClient()
	mock.getFunc = func(ctx context.Context, key string, opts ...clientv3.OpOption) (*clientv3.GetResponse, error) {
		return nil, errors.New("etcd down")
	}
	reg := NewEtcdRegistry(mock, testConfig(), "docker-host", testLogger())
	m := &countingMetrics{}
	reg.SetMetrics(m)

	if _, err := reg.List(context.Background()); err == nil {
		t.Fatal("expected List to return an error")
	}
	if etcdErrors, _ := m.snapshot(); etcdErrors != 1 {
		t.Errorf("expected 1 etcd error counted, got %d", etcdErrors)
	}
}

func TestEtcdRegistry_Metrics_LockFailureOnTimeout(t *testing.T) {
	mock := newMockEtcdClient()
	// Never let the lock be acquired so acquisition times out.
	mock.txnFunc = func(ctx context.Context) clientv3.Txn {
		return &mockTxn{ctx: ctx, commitFunc: func() (*clientv3.TxnResponse, error) {
			return &clientv3.TxnResponse{Succeeded: false}, nil
		}}
	}
	cfg := testConfig()
	cfg.LockTimeout = 0.05
	cfg.LockRetryInterval = 0.01
	reg := NewEtcdRegistry(mock, cfg, "docker-host", testLogger())
	m := &countingMetrics{}
	reg.SetMetrics(m)

	err := reg.LockTransaction(context.Background(), []string{"k"}, func() error { return nil })
	if err == nil {
		t.Fatal("expected lock acquisition to fail")
	}
	if _, lockFailures := m.snapshot(); lockFailures != 1 {
		t.Errorf("expected 1 lock failure counted, got %d", lockFailures)
	}
}

func TestEtcdRegistry_Metrics_UnsetIsNoop(t *testing.T) {
	mock := newMockEtcdClient()
	mock.getFunc = func(ctx context.Context, key string, opts ...clientv3.OpOption) (*clientv3.GetResponse, error) {
		return nil, errors.New("etcd down")
	}
	reg := NewEtcdRegistry(mock, testConfig(), "docker-host", testLogger())

	// No metrics set: must not panic.
	if _, err := reg.List(context.Background()); err == nil {
		t.Fatal("expected List to return an error")
	}
}
