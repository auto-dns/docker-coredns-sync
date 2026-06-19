package registry

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/auto-dns/docker-coredns-sync/internal/config"
	"github.com/auto-dns/docker-coredns-sync/internal/domain"
	"github.com/rs/zerolog"
	"go.etcd.io/etcd/api/v3/mvccpb"
	clientv3 "go.etcd.io/etcd/client/v3"
)

func testLogger() zerolog.Logger {
	return zerolog.New(io.Discard)
}

func testConfig() *config.EtcdConfig {
	return &config.EtcdConfig{
		Endpoints:         []string{"http://localhost:2379"},
		PathPrefix:        "/skydns",
		LockTTL:           5.0,
		LockTimeout:       2.0,
		LockRetryInterval: 0.1,
	}
}

func makeIntent(name, value string, kind domain.RecordKind) *domain.RecordIntent {
	var rec domain.Record
	switch kind {
	case domain.RecordA:
		rec, _ = domain.NewA(name, value)
	case domain.RecordAAAA:
		rec, _ = domain.NewAAAA(name, value)
	case domain.RecordCNAME:
		rec, _ = domain.NewCNAME(name, value)
	}
	return &domain.RecordIntent{
		ContainerId:   "test-container-123",
		ContainerName: "test-app",
		Created:       time.Now(),
		Hostname:      "docker-host",
		Force:         false,
		Record:        rec,
	}
}

func TestNewEtcdRegistry(t *testing.T) {
	mock := newMockEtcdClient()
	cfg := testConfig()

	reg := NewEtcdRegistry(mock, cfg, "test-host", 0, testLogger())

	if reg == nil {
		t.Fatal("expected non-nil registry")
	}
	if reg.client != mock {
		t.Error("expected client to be set")
	}
	if reg.hostname != "test-host" {
		t.Errorf("expected hostname 'test-host', got %q", reg.hostname)
	}
}

func TestEtcdRegistry_Register_CreatesKey(t *testing.T) {
	mock := newMockEtcdClient()
	cfg := testConfig()
	reg := NewEtcdRegistry(mock, cfg, "docker-host", 0, testLogger())

	ctx := context.Background()
	intent := makeIntent("app.example.com", "192.168.1.1", domain.RecordA)

	err := reg.Register(ctx, intent)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !mock.getCalled {
		t.Error("expected Get to be called for index lookup")
	}
	if !mock.putCalled {
		t.Error("expected Put to be called")
	}
	if len(mock.putKeys) != 1 {
		t.Fatalf("expected 1 put key, got %d", len(mock.putKeys))
	}

	// Key should be /skydns/com/example/app/x1
	expectedKeyPrefix := "/skydns/com/example/app/x"
	if !strings.HasPrefix(mock.putKeys[0], expectedKeyPrefix) {
		t.Errorf("expected key to start with %q, got %q", expectedKeyPrefix, mock.putKeys[0])
	}
}

func TestEtcdRegistry_Register_ValueContainsRecordData(t *testing.T) {
	mock := newMockEtcdClient()
	cfg := testConfig()
	reg := NewEtcdRegistry(mock, cfg, "docker-host", 0, testLogger())

	ctx := context.Background()
	intent := makeIntent("app.example.com", "192.168.1.1", domain.RecordA)

	err := reg.Register(ctx, intent)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(mock.putValues) != 1 {
		t.Fatalf("expected 1 put value, got %d", len(mock.putValues))
	}

	var wire etcdRecord
	if err := json.Unmarshal([]byte(mock.putValues[0]), &wire); err != nil {
		t.Fatalf("invalid JSON in put value: %v", err)
	}

	if wire.Host != "192.168.1.1" {
		t.Errorf("expected Host '192.168.1.1', got %q", wire.Host)
	}
	if wire.Kind != domain.RecordA {
		t.Errorf("expected Kind RecordA, got %v", wire.Kind)
	}
	if wire.OwnerHostname != "docker-host" {
		t.Errorf("expected OwnerHostname 'docker-host', got %q", wire.OwnerHostname)
	}
}

func TestEtcdRegistry_Register_IncrementsIndex(t *testing.T) {
	mock := newMockEtcdClient()
	cfg := testConfig()
	reg := NewEtcdRegistry(mock, cfg, "docker-host", 0, testLogger())

	// Simulate existing keys x1 and x2
	mock.getFunc = func(ctx context.Context, key string, opts ...clientv3.OpOption) (*clientv3.GetResponse, error) {
		return &clientv3.GetResponse{
			Kvs: []*mvccpb.KeyValue{
				{Key: []byte("/skydns/com/example/app/x1")},
				{Key: []byte("/skydns/com/example/app/x2")},
			},
		}, nil
	}

	ctx := context.Background()
	intent := makeIntent("app.example.com", "192.168.1.1", domain.RecordA)

	err := reg.Register(ctx, intent)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should use x3 since x1 and x2 exist
	expectedKey := "/skydns/com/example/app/x3"
	if mock.putKeys[0] != expectedKey {
		t.Errorf("expected key %q, got %q", expectedKey, mock.putKeys[0])
	}
}

func TestEtcdRegistry_Register_GetError(t *testing.T) {
	mock := newMockEtcdClient()
	mock.getFunc = func(ctx context.Context, key string, opts ...clientv3.OpOption) (*clientv3.GetResponse, error) {
		return nil, errors.New("etcd unavailable")
	}

	cfg := testConfig()
	reg := NewEtcdRegistry(mock, cfg, "docker-host", 0, testLogger())

	ctx := context.Background()
	intent := makeIntent("app.example.com", "192.168.1.1", domain.RecordA)

	err := reg.Register(ctx, intent)

	if err == nil {
		t.Error("expected error when Get fails")
	}
	if !strings.Contains(err.Error(), "etcd unavailable") {
		t.Errorf("expected error to contain 'etcd unavailable', got %v", err)
	}
}

func TestEtcdRegistry_Register_PutError(t *testing.T) {
	mock := newMockEtcdClient()
	mock.putFunc = func(ctx context.Context, key, val string, opts ...clientv3.OpOption) (*clientv3.PutResponse, error) {
		return nil, errors.New("write failed")
	}

	cfg := testConfig()
	reg := NewEtcdRegistry(mock, cfg, "docker-host", 0, testLogger())

	ctx := context.Background()
	intent := makeIntent("app.example.com", "192.168.1.1", domain.RecordA)

	err := reg.Register(ctx, intent)

	if err == nil {
		t.Error("expected error when Put fails")
	}
	if !strings.Contains(err.Error(), "write failed") {
		t.Errorf("expected error to contain 'write failed', got %v", err)
	}
}

func TestEtcdRegistry_Remove_DeletesMatchingKey(t *testing.T) {
	mock := newMockEtcdClient()
	cfg := testConfig()
	reg := NewEtcdRegistry(mock, cfg, "docker-host", 0, testLogger())

	// Simulate existing key with matching record
	existingValue, _ := json.Marshal(etcdRecord{
		Host:               "192.168.1.1",
		Kind:               domain.RecordA,
		OwnerHostname:      "docker-host",
		OwnerContainerId:   "test-container-123",
		OwnerContainerName: "test-app",
		Created:            time.Now(),
	})
	mock.getFunc = func(ctx context.Context, key string, opts ...clientv3.OpOption) (*clientv3.GetResponse, error) {
		return &clientv3.GetResponse{
			Kvs: []*mvccpb.KeyValue{
				{
					Key:   []byte("/skydns/com/example/app/x1"),
					Value: existingValue,
				},
			},
		}, nil
	}

	ctx := context.Background()
	intent := makeIntent("app.example.com", "192.168.1.1", domain.RecordA)

	err := reg.Remove(ctx, intent)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !mock.txnCalled {
		t.Error("expected Txn to be called for delete")
	}
}

func TestEtcdRegistry_Remove_NoMatchingKeys(t *testing.T) {
	mock := newMockEtcdClient()
	cfg := testConfig()
	reg := NewEtcdRegistry(mock, cfg, "docker-host", 0, testLogger())

	// No matching records
	mock.getFunc = func(ctx context.Context, key string, opts ...clientv3.OpOption) (*clientv3.GetResponse, error) {
		return &clientv3.GetResponse{Kvs: []*mvccpb.KeyValue{}}, nil
	}

	ctx := context.Background()
	intent := makeIntent("app.example.com", "192.168.1.1", domain.RecordA)

	err := reg.Remove(ctx, intent)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Txn should not be called if no keys to delete
	if mock.txnCalled {
		t.Error("expected Txn not to be called when no matching keys")
	}
}

func TestEtcdRegistry_Remove_DoesNotDeleteNonMatching(t *testing.T) {
	mock := newMockEtcdClient()
	cfg := testConfig()
	reg := NewEtcdRegistry(mock, cfg, "docker-host", 0, testLogger())

	// Existing key with different host
	existingValue, _ := json.Marshal(etcdRecord{
		Host:               "192.168.1.2", // Different IP
		Kind:               domain.RecordA,
		OwnerHostname:      "docker-host",
		OwnerContainerId:   "test-container-123",
		OwnerContainerName: "test-app",
		Created:            time.Now(),
	})
	mock.getFunc = func(ctx context.Context, key string, opts ...clientv3.OpOption) (*clientv3.GetResponse, error) {
		return &clientv3.GetResponse{
			Kvs: []*mvccpb.KeyValue{
				{
					Key:   []byte("/skydns/com/example/app/x1"),
					Value: existingValue,
				},
			},
		}, nil
	}

	ctx := context.Background()
	intent := makeIntent("app.example.com", "192.168.1.1", domain.RecordA) // Different IP

	err := reg.Remove(ctx, intent)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should not delete non-matching record
	if mock.txnCalled {
		t.Error("expected Txn not to be called for non-matching record")
	}
}

func TestEtcdRegistry_List_ReturnsAllRecords(t *testing.T) {
	mock := newMockEtcdClient()
	cfg := testConfig()
	reg := NewEtcdRegistry(mock, cfg, "docker-host", 0, testLogger())

	// Create test records
	record1, _ := json.Marshal(etcdRecord{
		Host:               "192.168.1.1",
		Kind:               domain.RecordA,
		OwnerHostname:      "docker-host",
		OwnerContainerId:   "container-1",
		OwnerContainerName: "app-1",
		Created:            time.Now(),
	})
	record2, _ := json.Marshal(etcdRecord{
		Host:               "192.168.1.2",
		Kind:               domain.RecordA,
		OwnerHostname:      "docker-host",
		OwnerContainerId:   "container-2",
		OwnerContainerName: "app-2",
		Created:            time.Now(),
	})

	mock.getFunc = func(ctx context.Context, key string, opts ...clientv3.OpOption) (*clientv3.GetResponse, error) {
		return &clientv3.GetResponse{
			Kvs: []*mvccpb.KeyValue{
				{Key: []byte("/skydns/com/example/app1/x1"), Value: record1},
				{Key: []byte("/skydns/com/example/app2/x1"), Value: record2},
			},
		}, nil
	}

	ctx := context.Background()
	intents, err := reg.List(ctx)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(intents) != 2 {
		t.Fatalf("expected 2 intents, got %d", len(intents))
	}
}

func TestEtcdRegistry_List_HandlesInvalidJSON(t *testing.T) {
	mock := newMockEtcdClient()
	cfg := testConfig()
	reg := NewEtcdRegistry(mock, cfg, "docker-host", 0, testLogger())

	validRecord, _ := json.Marshal(etcdRecord{
		Host:               "192.168.1.1",
		Kind:               domain.RecordA,
		OwnerHostname:      "docker-host",
		OwnerContainerId:   "container-1",
		OwnerContainerName: "app-1",
		Created:            time.Now(),
	})

	mock.getFunc = func(ctx context.Context, key string, opts ...clientv3.OpOption) (*clientv3.GetResponse, error) {
		return &clientv3.GetResponse{
			Kvs: []*mvccpb.KeyValue{
				{Key: []byte("/skydns/com/example/app1/x1"), Value: validRecord},
				{Key: []byte("/skydns/com/example/app2/x1"), Value: []byte("invalid json")},
			},
		}, nil
	}

	ctx := context.Background()
	intents, err := reg.List(ctx)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should only return the valid record
	if len(intents) != 1 {
		t.Fatalf("expected 1 intent (invalid skipped), got %d", len(intents))
	}
}

func TestEtcdRegistry_List_Error(t *testing.T) {
	mock := newMockEtcdClient()
	mock.getFunc = func(ctx context.Context, key string, opts ...clientv3.OpOption) (*clientv3.GetResponse, error) {
		return nil, errors.New("connection refused")
	}

	cfg := testConfig()
	reg := NewEtcdRegistry(mock, cfg, "docker-host", 0, testLogger())

	ctx := context.Background()
	_, err := reg.List(ctx)

	if err == nil {
		t.Error("expected error when Get fails")
	}
}

func TestEtcdRegistry_List_Empty(t *testing.T) {
	mock := newMockEtcdClient()
	cfg := testConfig()
	reg := NewEtcdRegistry(mock, cfg, "docker-host", 0, testLogger())

	ctx := context.Background()
	intents, err := reg.List(ctx)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(intents) != 0 {
		t.Errorf("expected 0 intents, got %d", len(intents))
	}
}

func TestEtcdRegistry_Close(t *testing.T) {
	mock := newMockEtcdClient()
	cfg := testConfig()
	reg := NewEtcdRegistry(mock, cfg, "docker-host", 0, testLogger())

	err := reg.Close()

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !mock.closeCalled {
		t.Error("expected Close to be called on client")
	}
}

func TestEtcdRegistry_Close_Error(t *testing.T) {
	mock := newMockEtcdClient()
	mock.closeFunc = func() error {
		return errors.New("close failed")
	}

	cfg := testConfig()
	reg := NewEtcdRegistry(mock, cfg, "docker-host", 0, testLogger())

	err := reg.Close()

	if err == nil {
		t.Error("expected error from Close")
	}
}

func TestEtcdRegistry_LockTransaction_Success(t *testing.T) {
	mock := newMockEtcdClient()
	cfg := testConfig()
	reg := NewEtcdRegistry(mock, cfg, "docker-host", 0, testLogger())

	executed := false
	ctx := context.Background()

	err := reg.LockTransaction(ctx, []string{"key1", "key2"}, func() error {
		executed = true
		return nil
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !executed {
		t.Error("expected function to be executed")
	}

	if !mock.grantCalled {
		t.Error("expected Grant to be called for lease")
	}
	if !mock.keepAliveCalled {
		t.Error("expected KeepAlive to be called")
	}
}

func TestEtcdRegistry_LockTransaction_FunctionError(t *testing.T) {
	mock := newMockEtcdClient()
	cfg := testConfig()
	reg := NewEtcdRegistry(mock, cfg, "docker-host", 0, testLogger())

	ctx := context.Background()

	err := reg.LockTransaction(ctx, []string{"key1"}, func() error {
		return errors.New("function error")
	})

	if err == nil {
		t.Error("expected error from function")
	}
	if !strings.Contains(err.Error(), "function error") {
		t.Errorf("expected 'function error', got %v", err)
	}

	// Locks should still be released
	if !mock.deleteCalled {
		t.Error("expected Delete to be called to release lock")
	}
	if !mock.revokeCalled {
		t.Error("expected Revoke to be called to release lease")
	}
}

func TestEtcdRegistry_LockTransaction_DeduplicatesKeys(t *testing.T) {
	mock := newMockEtcdClient()
	cfg := testConfig()
	reg := NewEtcdRegistry(mock, cfg, "docker-host", 0, testLogger())

	grantCount := 0
	mock.grantFunc = func(ctx context.Context, ttl int64) (*clientv3.LeaseGrantResponse, error) {
		grantCount++
		return &clientv3.LeaseGrantResponse{ID: clientv3.LeaseID(grantCount)}, nil
	}

	ctx := context.Background()

	err := reg.LockTransaction(ctx, []string{"key1", "key1", "key1"}, func() error {
		return nil
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should only create one lease for deduplicated keys
	if grantCount != 1 {
		t.Errorf("expected 1 grant call (deduplicated), got %d", grantCount)
	}
}

func TestEtcdRegistry_LockTransaction_ContextCancellation(t *testing.T) {
	mock := newMockEtcdClient()
	cfg := testConfig()
	reg := NewEtcdRegistry(mock, cfg, "docker-host", 0, testLogger())

	ctx, cancel := context.WithCancel(context.Background())

	// Cancel immediately after grant
	mock.grantFunc = func(ctx context.Context, ttl int64) (*clientv3.LeaseGrantResponse, error) {
		cancel()
		return &clientv3.LeaseGrantResponse{ID: 1}, nil
	}

	err := reg.LockTransaction(ctx, []string{"key1"}, func() error {
		return nil
	})

	if err == nil {
		t.Error("expected error from context cancellation")
	}
}

func TestEtcdRegistry_Register_DifferentRecordTypes(t *testing.T) {
	tests := []struct {
		name  string
		kind  domain.RecordKind
		value string
	}{
		{"A record", domain.RecordA, "192.168.1.1"},
		{"AAAA record", domain.RecordAAAA, "2001:db8::1"},
		{"CNAME record", domain.RecordCNAME, "target.example.com"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := newMockEtcdClient()
			cfg := testConfig()
			reg := NewEtcdRegistry(mock, cfg, "docker-host", 0, testLogger())

			ctx := context.Background()
			intent := makeIntent("app.example.com", tt.value, tt.kind)

			err := reg.Register(ctx, intent)

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			var wire etcdRecord
			json.Unmarshal([]byte(mock.putValues[0]), &wire)

			if wire.Kind != tt.kind {
				t.Errorf("expected Kind %v, got %v", tt.kind, wire.Kind)
			}
			if wire.Host != tt.value {
				t.Errorf("expected Host %q, got %q", tt.value, wire.Host)
			}
		})
	}
}

func TestEtcdRegistry_recordMatches(t *testing.T) {
	cfg := testConfig()
	reg := NewEtcdRegistry(nil, cfg, "docker-host", 0, testLogger())

	intent := makeIntent("app.example.com", "192.168.1.1", domain.RecordA)

	tests := []struct {
		name     string
		wire     etcdRecord
		expected bool
	}{
		{
			name: "exact match",
			wire: etcdRecord{
				Host:               "192.168.1.1",
				Kind:               domain.RecordA,
				OwnerHostname:      "docker-host",
				OwnerContainerId:   "test-container-123",
				OwnerContainerName: "test-app",
			},
			expected: true,
		},
		{
			name: "different host",
			wire: etcdRecord{
				Host:               "192.168.1.2",
				Kind:               domain.RecordA,
				OwnerHostname:      "docker-host",
				OwnerContainerId:   "test-container-123",
				OwnerContainerName: "test-app",
			},
			expected: false,
		},
		{
			name: "different kind",
			wire: etcdRecord{
				Host:               "192.168.1.1",
				Kind:               domain.RecordAAAA,
				OwnerHostname:      "docker-host",
				OwnerContainerId:   "test-container-123",
				OwnerContainerName: "test-app",
			},
			expected: false,
		},
		{
			name: "different owner hostname",
			wire: etcdRecord{
				Host:               "192.168.1.1",
				Kind:               domain.RecordA,
				OwnerHostname:      "other-host",
				OwnerContainerId:   "test-container-123",
				OwnerContainerName: "test-app",
			},
			expected: false,
		},
		{
			name: "different container name",
			wire: etcdRecord{
				Host:               "192.168.1.1",
				Kind:               domain.RecordA,
				OwnerHostname:      "docker-host",
				OwnerContainerId:   "test-container-123",
				OwnerContainerName: "other-app",
			},
			expected: false,
		},
		{
			name: "empty container id in intent allows match",
			wire: etcdRecord{
				Host:               "192.168.1.1",
				Kind:               domain.RecordA,
				OwnerHostname:      "docker-host",
				OwnerContainerId:   "any-container",
				OwnerContainerName: "test-app",
			},
			expected: false, // ContainerId in intent is set
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := reg.recordMatches(tt.wire, intent)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestEtcdRegistry_recordMatches_EmptyContainerId(t *testing.T) {
	cfg := testConfig()
	reg := NewEtcdRegistry(nil, cfg, "docker-host", 0, testLogger())

	rec, _ := domain.NewA("app.example.com", "192.168.1.1")
	intent := &domain.RecordIntent{
		ContainerId:   "", // Empty - should match any
		ContainerName: "test-app",
		Created:       time.Now(),
		Hostname:      "docker-host",
		Force:         false,
		Record:        rec,
	}

	wire := etcdRecord{
		Host:               "192.168.1.1",
		Kind:               domain.RecordA,
		OwnerHostname:      "docker-host",
		OwnerContainerId:   "any-container-id",
		OwnerContainerName: "test-app",
	}

	result := reg.recordMatches(wire, intent)

	if !result {
		t.Error("expected match when intent ContainerId is empty")
	}
}

// ============================================================================
// Additional Remove tests
// ============================================================================

func TestEtcdRegistry_Remove_GetError(t *testing.T) {
	mock := newMockEtcdClient()
	mock.getFunc = func(ctx context.Context, key string, opts ...clientv3.OpOption) (*clientv3.GetResponse, error) {
		return nil, errors.New("etcd unavailable")
	}

	cfg := testConfig()
	reg := NewEtcdRegistry(mock, cfg, "docker-host", 0, testLogger())

	ctx := context.Background()
	intent := makeIntent("app.example.com", "192.168.1.1", domain.RecordA)

	err := reg.Remove(ctx, intent)

	if err == nil {
		t.Error("expected error when Get fails")
	}
}

func TestEtcdRegistry_Remove_MultipleMatches(t *testing.T) {
	mock := newMockEtcdClient()
	cfg := testConfig()
	reg := NewEtcdRegistry(mock, cfg, "docker-host", 0, testLogger())

	// Multiple matching records
	existingValue, _ := json.Marshal(etcdRecord{
		Host:               "192.168.1.1",
		Kind:               domain.RecordA,
		OwnerHostname:      "docker-host",
		OwnerContainerId:   "test-container-123",
		OwnerContainerName: "test-app",
		Created:            time.Now(),
	})
	mock.getFunc = func(ctx context.Context, key string, opts ...clientv3.OpOption) (*clientv3.GetResponse, error) {
		return &clientv3.GetResponse{
			Kvs: []*mvccpb.KeyValue{
				{Key: []byte("/skydns/com/example/app/x1"), Value: existingValue},
				{Key: []byte("/skydns/com/example/app/x2"), Value: existingValue},
				{Key: []byte("/skydns/com/example/app/x3"), Value: existingValue},
			},
		}, nil
	}

	ctx := context.Background()
	intent := makeIntent("app.example.com", "192.168.1.1", domain.RecordA)

	err := reg.Remove(ctx, intent)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !mock.txnCalled {
		t.Error("expected Txn to be called for batch delete")
	}
}

func TestEtcdRegistry_Remove_SkipsInvalidJSON(t *testing.T) {
	mock := newMockEtcdClient()
	cfg := testConfig()
	reg := NewEtcdRegistry(mock, cfg, "docker-host", 0, testLogger())

	validValue, _ := json.Marshal(etcdRecord{
		Host:               "192.168.1.1",
		Kind:               domain.RecordA,
		OwnerHostname:      "docker-host",
		OwnerContainerId:   "test-container-123",
		OwnerContainerName: "test-app",
		Created:            time.Now(),
	})
	mock.getFunc = func(ctx context.Context, key string, opts ...clientv3.OpOption) (*clientv3.GetResponse, error) {
		return &clientv3.GetResponse{
			Kvs: []*mvccpb.KeyValue{
				{Key: []byte("/skydns/com/example/app/x1"), Value: []byte("invalid json")},
				{Key: []byte("/skydns/com/example/app/x2"), Value: validValue},
			},
		}, nil
	}

	ctx := context.Background()
	intent := makeIntent("app.example.com", "192.168.1.1", domain.RecordA)

	err := reg.Remove(ctx, intent)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should still delete the valid matching record
	if !mock.txnCalled {
		t.Error("expected Txn to be called")
	}
}

func TestEtcdRegistry_Remove_SkipsNonChildKeys(t *testing.T) {
	mock := newMockEtcdClient()
	cfg := testConfig()
	reg := NewEtcdRegistry(mock, cfg, "docker-host", 0, testLogger())

	existingValue, _ := json.Marshal(etcdRecord{
		Host:               "192.168.1.1",
		Kind:               domain.RecordA,
		OwnerHostname:      "docker-host",
		OwnerContainerId:   "test-container-123",
		OwnerContainerName: "test-app",
		Created:            time.Now(),
	})
	mock.getFunc = func(ctx context.Context, key string, opts ...clientv3.OpOption) (*clientv3.GetResponse, error) {
		return &clientv3.GetResponse{
			Kvs: []*mvccpb.KeyValue{
				// This key doesn't have the proper prefix+slash structure
				{Key: []byte("/skydns/com/example/app"), Value: existingValue},
				// This one does
				{Key: []byte("/skydns/com/example/app/x1"), Value: existingValue},
			},
		}, nil
	}

	ctx := context.Background()
	intent := makeIntent("app.example.com", "192.168.1.1", domain.RecordA)

	err := reg.Remove(ctx, intent)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !mock.txnCalled {
		t.Error("expected Txn to be called for valid child key")
	}
}

// ============================================================================
// Additional LockTransaction tests
// ============================================================================

func TestEtcdRegistry_LockTransaction_GrantError(t *testing.T) {
	mock := newMockEtcdClient()
	mock.grantFunc = func(ctx context.Context, ttl int64) (*clientv3.LeaseGrantResponse, error) {
		return nil, errors.New("grant failed")
	}

	cfg := testConfig()
	reg := NewEtcdRegistry(mock, cfg, "docker-host", 0, testLogger())

	ctx := context.Background()

	err := reg.LockTransaction(ctx, []string{"key1"}, func() error {
		return nil
	})

	if err == nil {
		t.Error("expected error when Grant fails")
	}
	if !strings.Contains(err.Error(), "failed to create lease") {
		t.Errorf("expected 'failed to create lease' error, got %v", err)
	}
}

func TestEtcdRegistry_LockTransaction_TxnError(t *testing.T) {
	mock := newMockEtcdClient()
	mock.txnFunc = func(ctx context.Context) clientv3.Txn {
		return &mockTxn{
			ctx: ctx,
			commitFunc: func() (*clientv3.TxnResponse, error) {
				return nil, errors.New("txn commit failed")
			},
		}
	}

	cfg := testConfig()
	reg := NewEtcdRegistry(mock, cfg, "docker-host", 0, testLogger())

	ctx := context.Background()

	err := reg.LockTransaction(ctx, []string{"key1"}, func() error {
		return nil
	})

	if err == nil {
		t.Error("expected error when Txn fails")
	}
}

func TestEtcdRegistry_LockTransaction_KeepAliveError(t *testing.T) {
	mock := newMockEtcdClient()
	mock.keepAliveFunc = func(ctx context.Context, id clientv3.LeaseID) (<-chan *clientv3.LeaseKeepAliveResponse, error) {
		return nil, errors.New("keepalive failed")
	}

	cfg := testConfig()
	reg := NewEtcdRegistry(mock, cfg, "docker-host", 0, testLogger())

	ctx := context.Background()

	err := reg.LockTransaction(ctx, []string{"key1"}, func() error {
		return nil
	})

	if err == nil {
		t.Error("expected error when KeepAlive fails")
	}
	if !strings.Contains(err.Error(), "keepalive") {
		t.Errorf("expected 'keepalive' error, got %v", err)
	}
}

func TestEtcdRegistry_LockTransaction_EmptyKeys(t *testing.T) {
	mock := newMockEtcdClient()
	cfg := testConfig()
	reg := NewEtcdRegistry(mock, cfg, "docker-host", 0, testLogger())

	executed := false
	ctx := context.Background()

	err := reg.LockTransaction(ctx, []string{}, func() error {
		executed = true
		return nil
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !executed {
		t.Error("expected function to be executed with empty keys")
	}

	// No locks needed for empty keys
	if mock.grantCalled {
		t.Error("expected Grant not to be called for empty keys")
	}
}

func TestEtcdRegistry_LockTransaction_MultipleKeys(t *testing.T) {
	mock := newMockEtcdClient()
	cfg := testConfig()
	reg := NewEtcdRegistry(mock, cfg, "docker-host", 0, testLogger())

	grantCount := 0
	mock.grantFunc = func(ctx context.Context, ttl int64) (*clientv3.LeaseGrantResponse, error) {
		grantCount++
		return &clientv3.LeaseGrantResponse{ID: clientv3.LeaseID(grantCount)}, nil
	}

	ctx := context.Background()

	err := reg.LockTransaction(ctx, []string{"key1", "key2", "key3"}, func() error {
		return nil
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should create 3 leases for 3 unique keys
	if grantCount != 3 {
		t.Errorf("expected 3 grant calls, got %d", grantCount)
	}
}

// ============================================================================
// Additional getNextIndexedKey tests
// ============================================================================

func TestEtcdRegistry_getNextIndexedKey_GapFilling(t *testing.T) {
	mock := newMockEtcdClient()
	cfg := testConfig()
	reg := NewEtcdRegistry(mock, cfg, "docker-host", 0, testLogger())

	// Simulate existing keys with gaps: x1, x3, x5
	mock.getFunc = func(ctx context.Context, key string, opts ...clientv3.OpOption) (*clientv3.GetResponse, error) {
		return &clientv3.GetResponse{
			Kvs: []*mvccpb.KeyValue{
				{Key: []byte("/skydns/com/example/app/x1")},
				{Key: []byte("/skydns/com/example/app/x3")},
				{Key: []byte("/skydns/com/example/app/x5")},
			},
		}, nil
	}

	ctx := context.Background()
	key, err := reg.getNextIndexedKey(ctx, "app.example.com")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should fill the gap at x2
	expected := "/skydns/com/example/app/x2"
	if key != expected {
		t.Errorf("expected key %q (gap fill), got %q", expected, key)
	}
}

func TestEtcdRegistry_getNextIndexedKey_NonNumericSuffix(t *testing.T) {
	mock := newMockEtcdClient()
	cfg := testConfig()
	reg := NewEtcdRegistry(mock, cfg, "docker-host", 0, testLogger())

	// Keys with non-numeric suffixes should be ignored
	mock.getFunc = func(ctx context.Context, key string, opts ...clientv3.OpOption) (*clientv3.GetResponse, error) {
		return &clientv3.GetResponse{
			Kvs: []*mvccpb.KeyValue{
				{Key: []byte("/skydns/com/example/app/xabc")},
				{Key: []byte("/skydns/com/example/app/foo")},
			},
		}, nil
	}

	ctx := context.Background()
	key, err := reg.getNextIndexedKey(ctx, "app.example.com")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should start at x1 since no valid numeric indices exist
	expected := "/skydns/com/example/app/x1"
	if key != expected {
		t.Errorf("expected key %q, got %q", expected, key)
	}
}

// ============================================================================
// Additional coverage tests
// ============================================================================

func TestEtcdRegistry_Remove_BatchDeleteError(t *testing.T) {
	mock := newMockEtcdClient()
	cfg := testConfig()
	reg := NewEtcdRegistry(mock, cfg, "docker-host", 0, testLogger())

	intent := makeIntent("app.example.com", "192.168.1.1", domain.RecordA)
	value, _ := json.Marshal(etcdRecord{
		Host:               "192.168.1.1",
		Kind:               domain.RecordA,
		OwnerHostname:      "docker-host",
		OwnerContainerId:   "test-container-123",
		OwnerContainerName: "test-app",
	})

	mock.getFunc = func(ctx context.Context, key string, opts ...clientv3.OpOption) (*clientv3.GetResponse, error) {
		return &clientv3.GetResponse{
			Kvs: []*mvccpb.KeyValue{
				{Key: []byte("/skydns/com/example/app/x1"), Value: value},
			},
		}, nil
	}

	// Make Txn.Commit fail
	mock.txnFunc = func(ctx context.Context) clientv3.Txn {
		return &mockTxn{
			commitFunc: func() (*clientv3.TxnResponse, error) {
				return nil, errors.New("batch delete failed")
			},
		}
	}

	ctx := context.Background()
	err := reg.Remove(ctx, intent)

	if err == nil {
		t.Fatal("expected error when batch delete fails")
	}
	if !strings.Contains(err.Error(), "batch delete") {
		t.Errorf("expected 'batch delete' error, got %v", err)
	}
}

func TestEtcdRegistry_LockTransaction_ContextCancelDuringRetry(t *testing.T) {
	mock := newMockEtcdClient()
	cfg := &config.EtcdConfig{
		Endpoints:         []string{"http://localhost:2379"},
		PathPrefix:        "/skydns",
		LockTTL:           5.0,
		LockTimeout:       5.0, // Long timeout
		LockRetryInterval: 0.01,
	}
	reg := NewEtcdRegistry(mock, cfg, "docker-host", 0, testLogger())

	// Make Txn always fail to acquire (Succeeded = false)
	mock.txnFunc = func(ctx context.Context) clientv3.Txn {
		return &mockTxn{
			commitFunc: func() (*clientv3.TxnResponse, error) {
				return &clientv3.TxnResponse{Succeeded: false}, nil
			},
		}
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Cancel after a short delay
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	err := reg.LockTransaction(ctx, []string{"key1"}, func() error {
		return nil
	})

	if err == nil {
		t.Fatal("expected error when context is cancelled")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

func TestEtcdRegistry_LockTransaction_AcquireTimeout(t *testing.T) {
	mock := newMockEtcdClient()
	cfg := &config.EtcdConfig{
		Endpoints:         []string{"http://localhost:2379"},
		PathPrefix:        "/skydns",
		LockTTL:           5.0,
		LockTimeout:       0.05, // Very short timeout
		LockRetryInterval: 0.01,
	}
	reg := NewEtcdRegistry(mock, cfg, "docker-host", 0, testLogger())

	// Make Txn always fail to acquire (Succeeded = false)
	mock.txnFunc = func(ctx context.Context) clientv3.Txn {
		return &mockTxn{
			commitFunc: func() (*clientv3.TxnResponse, error) {
				return &clientv3.TxnResponse{Succeeded: false}, nil
			},
		}
	}

	ctx := context.Background()

	err := reg.LockTransaction(ctx, []string{"key1"}, func() error {
		return nil
	})

	if err == nil {
		t.Fatal("expected error when lock acquire times out")
	}
	if !strings.Contains(err.Error(), "failed to acquire lock") {
		t.Errorf("expected 'failed to acquire lock' error, got %v", err)
	}
}

func TestEtcdRegistry_LockTransaction_RevokeTimeoutError(t *testing.T) {
	mock := newMockEtcdClient()
	cfg := &config.EtcdConfig{
		Endpoints:         []string{"http://localhost:2379"},
		PathPrefix:        "/skydns",
		LockTTL:           5.0,
		LockTimeout:       0.05,
		LockRetryInterval: 0.01,
	}
	reg := NewEtcdRegistry(mock, cfg, "docker-host", 0, testLogger())

	// Make Txn fail to acquire but revoke also fails
	mock.txnFunc = func(ctx context.Context) clientv3.Txn {
		return &mockTxn{
			commitFunc: func() (*clientv3.TxnResponse, error) {
				return &clientv3.TxnResponse{Succeeded: false}, nil
			},
		}
	}
	mock.revokeFunc = func(ctx context.Context, id clientv3.LeaseID) (*clientv3.LeaseRevokeResponse, error) {
		return nil, errors.New("revoke failed")
	}

	ctx := context.Background()

	err := reg.LockTransaction(ctx, []string{"key1"}, func() error {
		return nil
	})

	// Should still get the acquire timeout error
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "failed to acquire lock") {
		t.Errorf("expected 'failed to acquire lock' error, got %v", err)
	}
}

func TestEtcdRegistry_LockTransaction_DeleteAndRevokeErrorsDuringRelease(t *testing.T) {
	mock := newMockEtcdClient()
	cfg := testConfig()
	reg := NewEtcdRegistry(mock, cfg, "docker-host", 0, testLogger())

	executed := false

	// Track calls to enable error injection after lock is acquired
	txnCallCount := 0
	mock.txnFunc = func(ctx context.Context) clientv3.Txn {
		txnCallCount++
		return &mockTxn{
			commitFunc: func() (*clientv3.TxnResponse, error) {
				return &clientv3.TxnResponse{Succeeded: true}, nil
			},
		}
	}

	// Make Delete and Revoke fail
	mock.deleteFunc = func(ctx context.Context, key string, opts ...clientv3.OpOption) (*clientv3.DeleteResponse, error) {
		return nil, errors.New("delete failed")
	}
	mock.revokeFunc = func(ctx context.Context, id clientv3.LeaseID) (*clientv3.LeaseRevokeResponse, error) {
		return nil, errors.New("revoke failed")
	}

	ctx := context.Background()

	err := reg.LockTransaction(ctx, []string{"key1"}, func() error {
		executed = true
		return nil
	})

	// Function should still execute
	if !executed {
		t.Error("expected function to execute")
	}

	// No error returned because delete/revoke errors are logged but don't fail the transaction
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestEtcdRegistry_getNextIndexedKey_ConsecutiveIndices(t *testing.T) {
	mock := newMockEtcdClient()
	cfg := testConfig()
	reg := NewEtcdRegistry(mock, cfg, "docker-host", 0, testLogger())

	// Simulate existing keys: x1, x2, x3 (consecutive)
	mock.getFunc = func(ctx context.Context, key string, opts ...clientv3.OpOption) (*clientv3.GetResponse, error) {
		return &clientv3.GetResponse{
			Kvs: []*mvccpb.KeyValue{
				{Key: []byte("/skydns/com/example/app/x1")},
				{Key: []byte("/skydns/com/example/app/x2")},
				{Key: []byte("/skydns/com/example/app/x3")},
			},
		}, nil
	}

	ctx := context.Background()
	key, err := reg.getNextIndexedKey(ctx, "app.example.com")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should use x4
	expected := "/skydns/com/example/app/x4"
	if key != expected {
		t.Errorf("expected key %q, got %q", expected, key)
	}
}

func TestEtcdRegistry_getNextIndexedKey_KeyWithoutSlash(t *testing.T) {
	mock := newMockEtcdClient()
	cfg := testConfig()
	reg := NewEtcdRegistry(mock, cfg, "docker-host", 0, testLogger())

	// Keys that don't have the base+/ prefix should be skipped
	mock.getFunc = func(ctx context.Context, key string, opts ...clientv3.OpOption) (*clientv3.GetResponse, error) {
		return &clientv3.GetResponse{
			Kvs: []*mvccpb.KeyValue{
				{Key: []byte("/skydns/com/example/app")},           // Exact base, no slash
				{Key: []byte("/skydns/com/example/appx1")},         // No slash before x1
				{Key: []byte("/skydns/com/example/app/x1")},        // Valid key
				{Key: []byte("/skydns/com/example/app/nested/x2")}, // Nested path (suffix contains /)
			},
		}, nil
	}

	ctx := context.Background()
	key, err := reg.getNextIndexedKey(ctx, "app.example.com")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should pick x2 since only x1 is valid; nested path suffix is "nested/x2" which doesn't parse
	expected := "/skydns/com/example/app/x2"
	if key != expected {
		t.Errorf("expected key %q, got %q", expected, key)
	}
}

func TestEtcdRegistry_Remove_ContainerIdEmpty(t *testing.T) {
	mock := newMockEtcdClient()
	cfg := testConfig()
	reg := NewEtcdRegistry(mock, cfg, "docker-host", 0, testLogger())

	// Create intent with empty container ID (wildcard match)
	intent := &domain.RecordIntent{
		ContainerId:   "", // Empty - should match any container ID
		ContainerName: "test-app",
		Created:       time.Now(),
		Hostname:      "docker-host",
		Force:         false,
		Record:        domain.Record{Kind: domain.RecordA, Name: "app.example.com", Value: "192.168.1.1"},
	}

	value, _ := json.Marshal(etcdRecord{
		Host:               "192.168.1.1",
		Kind:               domain.RecordA,
		OwnerHostname:      "docker-host",
		OwnerContainerId:   "some-container-id",
		OwnerContainerName: "test-app",
	})

	mock.getFunc = func(ctx context.Context, key string, opts ...clientv3.OpOption) (*clientv3.GetResponse, error) {
		return &clientv3.GetResponse{
			Kvs: []*mvccpb.KeyValue{
				{Key: []byte("/skydns/com/example/app/x1"), Value: value},
			},
		}, nil
	}

	ctx := context.Background()
	err := reg.Remove(ctx, intent)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should match and trigger a transaction (delete)
	if !mock.txnCalled {
		t.Error("expected Txn to be called for delete")
	}
}
