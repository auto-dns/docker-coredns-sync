package registry

import (
	"context"
	"sync"

	"go.etcd.io/etcd/api/v3/mvccpb"
	clientv3 "go.etcd.io/etcd/client/v3"
)

type mockTxn struct {
	ctx     context.Context
	ifCmps  []clientv3.Cmp
	thenOps []clientv3.Op
	elseOps []clientv3.Op

	commitFunc func() (*clientv3.TxnResponse, error)
}

func (m *mockTxn) If(cs ...clientv3.Cmp) clientv3.Txn {
	m.ifCmps = cs
	return m
}

func (m *mockTxn) Then(ops ...clientv3.Op) clientv3.Txn {
	m.thenOps = ops
	return m
}

func (m *mockTxn) Else(ops ...clientv3.Op) clientv3.Txn {
	m.elseOps = ops
	return m
}

func (m *mockTxn) Commit() (*clientv3.TxnResponse, error) {
	if m.commitFunc != nil {
		return m.commitFunc()
	}
	return &clientv3.TxnResponse{Succeeded: true}, nil
}

type mockEtcdClient struct {
	mu sync.Mutex

	getFunc       func(ctx context.Context, key string, opts ...clientv3.OpOption) (*clientv3.GetResponse, error)
	putFunc       func(ctx context.Context, key, val string, opts ...clientv3.OpOption) (*clientv3.PutResponse, error)
	deleteFunc    func(ctx context.Context, key string, opts ...clientv3.OpOption) (*clientv3.DeleteResponse, error)
	grantFunc     func(ctx context.Context, ttl int64) (*clientv3.LeaseGrantResponse, error)
	txnFunc       func(ctx context.Context) clientv3.Txn
	revokeFunc    func(ctx context.Context, id clientv3.LeaseID) (*clientv3.LeaseRevokeResponse, error)
	keepAliveFunc func(ctx context.Context, id clientv3.LeaseID) (<-chan *clientv3.LeaseKeepAliveResponse, error)
	closeFunc     func() error

	getCalled       bool
	putCalled       bool
	deleteCalled    bool
	grantCalled     bool
	txnCalled       bool
	revokeCalled    bool
	keepAliveCalled bool
	closeCalled     bool

	putKeys   []string
	putValues []string
}

func newMockEtcdClient() *mockEtcdClient {
	return &mockEtcdClient{
		putKeys:   []string{},
		putValues: []string{},
	}
}

func (m *mockEtcdClient) Get(ctx context.Context, key string, opts ...clientv3.OpOption) (*clientv3.GetResponse, error) {
	m.mu.Lock()
	m.getCalled = true
	m.mu.Unlock()

	if m.getFunc != nil {
		return m.getFunc(ctx, key, opts...)
	}
	return &clientv3.GetResponse{Kvs: []*mvccpb.KeyValue{}}, nil
}

func (m *mockEtcdClient) Put(ctx context.Context, key, val string, opts ...clientv3.OpOption) (*clientv3.PutResponse, error) {
	m.mu.Lock()
	m.putCalled = true
	m.putKeys = append(m.putKeys, key)
	m.putValues = append(m.putValues, val)
	m.mu.Unlock()

	if m.putFunc != nil {
		return m.putFunc(ctx, key, val, opts...)
	}
	return &clientv3.PutResponse{}, nil
}

func (m *mockEtcdClient) Delete(ctx context.Context, key string, opts ...clientv3.OpOption) (*clientv3.DeleteResponse, error) {
	m.mu.Lock()
	m.deleteCalled = true
	m.mu.Unlock()

	if m.deleteFunc != nil {
		return m.deleteFunc(ctx, key, opts...)
	}
	return &clientv3.DeleteResponse{}, nil
}

func (m *mockEtcdClient) Grant(ctx context.Context, ttl int64) (*clientv3.LeaseGrantResponse, error) {
	m.mu.Lock()
	m.grantCalled = true
	m.mu.Unlock()

	if m.grantFunc != nil {
		return m.grantFunc(ctx, ttl)
	}
	return &clientv3.LeaseGrantResponse{ID: 1}, nil
}

func (m *mockEtcdClient) Txn(ctx context.Context) clientv3.Txn {
	m.mu.Lock()
	m.txnCalled = true
	m.mu.Unlock()

	if m.txnFunc != nil {
		return m.txnFunc(ctx)
	}
	return &mockTxn{ctx: ctx}
}

func (m *mockEtcdClient) Revoke(ctx context.Context, id clientv3.LeaseID) (*clientv3.LeaseRevokeResponse, error) {
	m.mu.Lock()
	m.revokeCalled = true
	m.mu.Unlock()

	if m.revokeFunc != nil {
		return m.revokeFunc(ctx, id)
	}
	return &clientv3.LeaseRevokeResponse{}, nil
}

func (m *mockEtcdClient) KeepAlive(ctx context.Context, id clientv3.LeaseID) (<-chan *clientv3.LeaseKeepAliveResponse, error) {
	m.mu.Lock()
	m.keepAliveCalled = true
	m.mu.Unlock()

	if m.keepAliveFunc != nil {
		return m.keepAliveFunc(ctx, id)
	}
	ch := make(chan *clientv3.LeaseKeepAliveResponse)
	go func() {
		<-ctx.Done()
		close(ch)
	}()
	return ch, nil
}

func (m *mockEtcdClient) Close() error {
	m.mu.Lock()
	m.closeCalled = true
	m.mu.Unlock()

	if m.closeFunc != nil {
		return m.closeFunc()
	}
	return nil
}

func (m *mockEtcdClient) reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.getCalled = false
	m.putCalled = false
	m.deleteCalled = false
	m.grantCalled = false
	m.txnCalled = false
	m.revokeCalled = false
	m.keepAliveCalled = false
	m.closeCalled = false
	m.putKeys = []string{}
	m.putValues = []string{}
}
