package registry

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/auto-dns/docker-coredns-sync/internal/domain"
	"go.etcd.io/etcd/api/v3/mvccpb"
	clientv3 "go.etcd.io/etcd/client/v3"
)

func ownedRecordKV(key, owner, host string) *mvccpb.KeyValue {
	b, _ := json.Marshal(etcdRecord{
		Host:          host,
		Kind:          domain.RecordA,
		OwnerHostname: owner,
	})
	return &mvccpb.KeyValue{Key: []byte(key), Value: b}
}

func TestEtcdRegistry_DecommissionHost_DeletesMarkerAndOwnedRecords(t *testing.T) {
	mock := newMockEtcdClient()

	var deletedKey string
	mock.deleteFunc = func(ctx context.Context, key string, opts ...clientv3.OpOption) (*clientv3.DeleteResponse, error) {
		deletedKey = key
		return &clientv3.DeleteResponse{}, nil
	}
	mock.getFunc = func(ctx context.Context, key string, opts ...clientv3.OpOption) (*clientv3.GetResponse, error) {
		return &clientv3.GetResponse{Kvs: []*mvccpb.KeyValue{
			ownedRecordKV("/skydns/com/example/a/x1", "dead-host", "10.0.0.1"),
			ownedRecordKV("/skydns/com/example/b/x1", "dead-host", "10.0.0.2"),
			ownedRecordKV("/skydns/com/example/c/x1", "live-host", "10.0.0.3"),
		}}, nil
	}
	reg := NewEtcdRegistry(mock, testConfig(), "ops-host", 0, testLogger())

	n, err := reg.DecommissionHost(context.Background(), "dead-host")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 2 {
		t.Errorf("expected 2 records deleted (only dead-host's), got %d", n)
	}
	if deletedKey != "/docker-coredns-sync/heartbeat/dead-host" {
		t.Errorf("expected heartbeat marker deleted, got %q", deletedKey)
	}
	if !mock.txnCalled {
		t.Error("expected a txn batch delete for the owned records")
	}
}

func TestEtcdRegistry_DecommissionHost_NoRecords(t *testing.T) {
	mock := newMockEtcdClient()
	reg := NewEtcdRegistry(mock, testConfig(), "ops-host", 0, testLogger())

	n, err := reg.DecommissionHost(context.Background(), "dead-host")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 0 {
		t.Errorf("expected 0 records deleted, got %d", n)
	}
	if !mock.deleteCalled {
		t.Error("expected the heartbeat marker to be deleted even with no records")
	}
}

func TestEtcdRegistry_DecommissionHost_MarkerDeleteError(t *testing.T) {
	mock := newMockEtcdClient()
	mock.deleteFunc = func(ctx context.Context, key string, opts ...clientv3.OpOption) (*clientv3.DeleteResponse, error) {
		return nil, errors.New("boom")
	}
	reg := NewEtcdRegistry(mock, testConfig(), "ops-host", 0, testLogger())

	if _, err := reg.DecommissionHost(context.Background(), "dead-host"); err == nil {
		t.Fatal("expected error when the heartbeat marker delete fails")
	}
}

func TestEtcdRegistry_DecommissionHost_ListError(t *testing.T) {
	mock := newMockEtcdClient()
	mock.getFunc = func(ctx context.Context, key string, opts ...clientv3.OpOption) (*clientv3.GetResponse, error) {
		return nil, errors.New("boom")
	}
	reg := NewEtcdRegistry(mock, testConfig(), "ops-host", 0, testLogger())

	if _, err := reg.DecommissionHost(context.Background(), "dead-host"); err == nil {
		t.Fatal("expected error when listing records fails")
	}
}

func TestEtcdRegistry_DecommissionHost_BatchDeleteError(t *testing.T) {
	mock := newMockEtcdClient()
	mock.getFunc = func(ctx context.Context, key string, opts ...clientv3.OpOption) (*clientv3.GetResponse, error) {
		return &clientv3.GetResponse{Kvs: []*mvccpb.KeyValue{
			ownedRecordKV("/skydns/com/example/a/x1", "dead-host", "10.0.0.1"),
		}}, nil
	}
	mock.txnFunc = func(ctx context.Context) clientv3.Txn {
		return &mockTxn{ctx: ctx, commitFunc: func() (*clientv3.TxnResponse, error) {
			return nil, errors.New("boom")
		}}
	}
	reg := NewEtcdRegistry(mock, testConfig(), "ops-host", 0, testLogger())

	n, err := reg.DecommissionHost(context.Background(), "dead-host")
	if err == nil {
		t.Fatal("expected error when a batch delete fails")
	}
	if n != 0 {
		t.Errorf("expected 0 records counted as deleted on failure, got %d", n)
	}
}
