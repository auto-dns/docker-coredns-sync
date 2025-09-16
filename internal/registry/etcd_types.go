package registry

import (
	"context"

	clientv3 "go.etcd.io/etcd/client/v3"
)

type heldLease struct {
	lockKey string
	lease   clientv3.LeaseID
	cancel  context.CancelFunc
}
