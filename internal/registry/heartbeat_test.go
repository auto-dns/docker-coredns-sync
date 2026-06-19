package registry

import (
	"context"
	"errors"
	"testing"

	"go.etcd.io/etcd/api/v3/mvccpb"
	clientv3 "go.etcd.io/etcd/client/v3"
)

func TestEtcdRegistry_StartHeartbeat_DisabledWritesStaticMarker(t *testing.T) {
	mock := newMockEtcdClient()
	reg := NewEtcdRegistry(mock, testConfig(), "docker-host", 0, testLogger())

	if err := reg.StartHeartbeat(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// A disabled host publishes a persistent (unleased) opt-out marker: a Put
	// with no Grant and no KeepAlive.
	if mock.grantCalled {
		t.Error("expected no lease grant when heartbeat is disabled")
	}
	if mock.keepAliveCalled {
		t.Error("expected no keepalive when heartbeat is disabled")
	}
	if len(mock.putKeys) != 1 {
		t.Fatalf("expected 1 put (the static marker), got %d", len(mock.putKeys))
	}
	wantKey := "/docker-coredns-sync/heartbeat/docker-host"
	if mock.putKeys[0] != wantKey {
		t.Errorf("expected marker key %q, got %q", wantKey, mock.putKeys[0])
	}
	if mock.putValues[0] != heartbeatStaticValue {
		t.Errorf("expected marker value %q, got %q", heartbeatStaticValue, mock.putValues[0])
	}
	if reg.hbActive {
		t.Error("expected hbActive to be false for a disabled host (it must not run GC)")
	}
}

func TestEtcdRegistry_StartHeartbeat_DisabledMarkerPutError(t *testing.T) {
	mock := newMockEtcdClient()
	mock.putFunc = func(ctx context.Context, key, val string, opts ...clientv3.OpOption) (*clientv3.PutResponse, error) {
		return nil, errors.New("boom")
	}
	reg := NewEtcdRegistry(mock, testConfig(), "docker-host", 0, testLogger())

	if err := reg.StartHeartbeat(context.Background()); err == nil {
		t.Fatal("expected error when the static marker put fails")
	}
}

func TestEtcdRegistry_StartHeartbeat_WritesLeasedKey(t *testing.T) {
	mock := newMockEtcdClient()
	reg := NewEtcdRegistry(mock, testConfig(), "docker-host", 30, testLogger())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := reg.StartHeartbeat(ctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !mock.grantCalled {
		t.Error("expected Grant to be called")
	}
	if !mock.keepAliveCalled {
		t.Error("expected KeepAlive to be called")
	}
	if len(mock.putKeys) != 1 {
		t.Fatalf("expected 1 put, got %d", len(mock.putKeys))
	}
	wantKey := "/docker-coredns-sync/heartbeat/docker-host"
	if mock.putKeys[0] != wantKey {
		t.Errorf("expected heartbeat key %q, got %q", wantKey, mock.putKeys[0])
	}
	if mock.putValues[0] != "docker-host" {
		t.Errorf("expected heartbeat value 'docker-host', got %q", mock.putValues[0])
	}
	if reg.hbLease == 0 {
		t.Error("expected heartbeat lease to be recorded")
	}
	if !reg.hbActive {
		t.Error("expected hbActive to be true after a successful leased heartbeat")
	}
}

func TestEtcdRegistry_StartHeartbeat_GrantError(t *testing.T) {
	mock := newMockEtcdClient()
	mock.grantFunc = func(ctx context.Context, ttl int64) (*clientv3.LeaseGrantResponse, error) {
		return nil, errors.New("boom")
	}
	reg := NewEtcdRegistry(mock, testConfig(), "docker-host", 30, testLogger())

	if err := reg.StartHeartbeat(context.Background()); err == nil {
		t.Fatal("expected error when grant fails")
	}
	if reg.hbActive {
		t.Error("expected hbActive to stay false after a failed start")
	}
}

func TestEtcdRegistry_StartHeartbeat_PutErrorRevokes(t *testing.T) {
	mock := newMockEtcdClient()
	mock.putFunc = func(ctx context.Context, key, val string, opts ...clientv3.OpOption) (*clientv3.PutResponse, error) {
		return nil, errors.New("boom")
	}
	reg := NewEtcdRegistry(mock, testConfig(), "docker-host", 30, testLogger())

	if err := reg.StartHeartbeat(context.Background()); err == nil {
		t.Fatal("expected error when put fails")
	}
	if !mock.revokeCalled {
		t.Error("expected lease to be revoked after put failure")
	}
}

func TestEtcdRegistry_StartHeartbeat_KeepAliveErrorCleansUp(t *testing.T) {
	mock := newMockEtcdClient()
	mock.keepAliveFunc = func(ctx context.Context, id clientv3.LeaseID) (<-chan *clientv3.LeaseKeepAliveResponse, error) {
		return nil, errors.New("boom")
	}
	reg := NewEtcdRegistry(mock, testConfig(), "docker-host", 30, testLogger())

	if err := reg.StartHeartbeat(context.Background()); err == nil {
		t.Fatal("expected error when keepalive fails")
	}
	if !mock.deleteCalled {
		t.Error("expected heartbeat key to be deleted after keepalive failure")
	}
	if !mock.revokeCalled {
		t.Error("expected lease to be revoked after keepalive failure")
	}
}

func TestEtcdRegistry_Close_NoHeartbeatStarted(t *testing.T) {
	mock := newMockEtcdClient()
	reg := NewEtcdRegistry(mock, testConfig(), "docker-host", 0, testLogger())

	// stopHeartbeat must be a no-op (no delete/revoke) when no leased heartbeat
	// was started — including for a disabled host, whose persistent marker must
	// survive shutdown.
	if err := reg.Close(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mock.deleteCalled || mock.revokeCalled {
		t.Error("expected no heartbeat cleanup when no leased heartbeat was started")
	}
}

func TestEtcdRegistry_GetLiveHostnames_NotActiveReturnsNil(t *testing.T) {
	// Enabled by config, but StartHeartbeat was never run (or failed): the host
	// must not reap any peer's records.
	mock := newMockEtcdClient()
	mock.getFunc = func(ctx context.Context, key string, opts ...clientv3.OpOption) (*clientv3.GetResponse, error) {
		t.Error("Get must not be called when this host is not actively heartbeating")
		return &clientv3.GetResponse{}, nil
	}
	reg := NewEtcdRegistry(mock, testConfig(), "docker-host", 30, testLogger())

	live, err := reg.GetLiveHostnames(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if live != nil {
		t.Errorf("expected nil set when not actively heartbeating, got %v", live)
	}
}

func TestEtcdRegistry_GetLiveHostnames_DisabledReturnsNil(t *testing.T) {
	mock := newMockEtcdClient()
	reg := NewEtcdRegistry(mock, testConfig(), "docker-host", 0, testLogger())

	live, err := reg.GetLiveHostnames(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if live != nil {
		t.Errorf("expected nil set when heartbeat disabled, got %v", live)
	}
}

func TestEtcdRegistry_GetLiveHostnames_FailedStartLeavesGCDisabled(t *testing.T) {
	mock := newMockEtcdClient()
	mock.grantFunc = func(ctx context.Context, ttl int64) (*clientv3.LeaseGrantResponse, error) {
		return nil, errors.New("boom")
	}
	reg := NewEtcdRegistry(mock, testConfig(), "docker-host", 30, testLogger())

	if err := reg.StartHeartbeat(context.Background()); err == nil {
		t.Fatal("expected StartHeartbeat to fail")
	}
	live, err := reg.GetLiveHostnames(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if live != nil {
		t.Errorf("expected GC disabled (nil set) after a failed heartbeat start, got %v", live)
	}
}

func TestEtcdRegistry_GetLiveHostnames_ParsesKeysAndIncludesSelf(t *testing.T) {
	mock := newMockEtcdClient()
	mock.getFunc = func(ctx context.Context, key string, opts ...clientv3.OpOption) (*clientv3.GetResponse, error) {
		return &clientv3.GetResponse{Kvs: []*mvccpb.KeyValue{
			{Key: []byte("/docker-coredns-sync/heartbeat/host-a")},
			{Key: []byte("/docker-coredns-sync/heartbeat/host-b")},
		}}, nil
	}
	reg := NewEtcdRegistry(mock, testConfig(), "docker-host", 30, testLogger())
	reg.hbActive = true // simulate a started heartbeat

	live, err := reg.GetLiveHostnames(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, want := range []string{"host-a", "host-b", "docker-host"} {
		if _, ok := live[want]; !ok {
			t.Errorf("expected %q in live set, got %v", want, live)
		}
	}
	if len(live) != 3 {
		t.Errorf("expected 3 live hosts, got %d (%v)", len(live), live)
	}
}

func TestEtcdRegistry_GetLiveHostnames_SkipsEmptySuffix(t *testing.T) {
	mock := newMockEtcdClient()
	mock.getFunc = func(ctx context.Context, key string, opts ...clientv3.OpOption) (*clientv3.GetResponse, error) {
		return &clientv3.GetResponse{Kvs: []*mvccpb.KeyValue{
			{Key: []byte("/docker-coredns-sync/heartbeat/")}, // malformed: empty hostname
			{Key: []byte("/docker-coredns-sync/heartbeat/host-a")},
		}}, nil
	}
	reg := NewEtcdRegistry(mock, testConfig(), "docker-host", 30, testLogger())
	reg.hbActive = true

	live, err := reg.GetLiveHostnames(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := live[""]; ok {
		t.Error("expected empty hostname to be skipped")
	}
	if _, ok := live["host-a"]; !ok {
		t.Error("expected host-a to be present")
	}
}

func TestEtcdRegistry_GetLiveHostnames_GetError(t *testing.T) {
	mock := newMockEtcdClient()
	mock.getFunc = func(ctx context.Context, key string, opts ...clientv3.OpOption) (*clientv3.GetResponse, error) {
		return nil, errors.New("boom")
	}
	reg := NewEtcdRegistry(mock, testConfig(), "docker-host", 30, testLogger())
	reg.hbActive = true

	if _, err := reg.GetLiveHostnames(context.Background()); err == nil {
		t.Fatal("expected error when get fails")
	}
}

func TestEtcdRegistry_Close_StopsHeartbeat(t *testing.T) {
	mock := newMockEtcdClient()
	reg := NewEtcdRegistry(mock, testConfig(), "docker-host", 30, testLogger())

	if err := reg.StartHeartbeat(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	mock.reset()

	if err := reg.Close(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !mock.deleteCalled {
		t.Error("expected heartbeat key to be deleted on close")
	}
	if !mock.revokeCalled {
		t.Error("expected heartbeat lease to be revoked on close")
	}
	if !mock.closeCalled {
		t.Error("expected underlying client to be closed")
	}
}
