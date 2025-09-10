package registry

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/auto-dns/docker-coredns-sync/internal/config"
	"github.com/auto-dns/docker-coredns-sync/internal/domain"
	"github.com/auto-dns/docker-coredns-sync/internal/util"
	"github.com/rs/zerolog"
	clientv3 "go.etcd.io/etcd/client/v3"
)

type EtcdRegistry struct {
	client   etcdClient
	cfg      *config.EtcdConfig
	hostname string
	logger   zerolog.Logger
}

func NewEtcdRegistry(client etcdClient, cfg *config.EtcdConfig, hostname string, logger zerolog.Logger) *EtcdRegistry {
	return &EtcdRegistry{
		client:   client,
		cfg:      cfg,
		hostname: hostname,
		logger:   logger,
	}
}

// getNextIndexedKey generates a new etcd key for a record based on its fully qualified domain name (fqdn).
func (er *EtcdRegistry) getNextIndexedKey(ctx context.Context, fqdn string) (string, error) {
	trimmed := strings.Trim(fqdn, ".")
	parts := strings.Split(trimmed, ".")
	parts_reversed := util.Reverse(parts)
	baseKey := fmt.Sprintf("%s/%s", er.cfg.PathPrefix, strings.Join(parts_reversed, "/"))
	existingIndices := make(map[int]struct{})

	resp, err := er.client.Get(ctx, baseKey, clientv3.WithPrefix())
	if err != nil {
		return "", err
	}
	for _, kv := range resp.Kvs {
		keyStr := string(kv.Key)
		// Suffix is after the last "/" (expected to be "x<number>")
		idx := strings.LastIndex(keyStr, "/")
		if idx < 0 {
			continue
		}
		suffix := keyStr[idx+1:]
		base := keyStr[:idx]

		if base != baseKey {
			continue
		}

		if strings.HasPrefix(suffix, "x") {
			numStr := suffix[1:]
			num, err := strconv.Atoi(numStr)
			if err == nil {
				existingIndices[num] = struct{}{}
			}
		}
	}
	index := 1
	for {
		if _, exists := existingIndices[index]; !exists {
			break
		}
		index++
	}
	return fmt.Sprintf("%s/x%d", baseKey, index), nil
}

// Register stores the record intent in etcd.
func (er *EtcdRegistry) Register(ctx context.Context, ri *domain.RecordIntent) error {
	fqdn := ri.Record.Name
	key, err := er.getNextIndexedKey(ctx, fqdn)
	if err != nil {
		return err
	}
	value, err := marshalEtcdValue(ri)
	if err != nil {
		return err
	}
	_, err = er.client.Put(ctx, key, value)
	return err
}

// Remove finds and deletes the etcd key that matches the record domain.
func (er *EtcdRegistry) Remove(ctx context.Context, ri *domain.RecordIntent) error {
	fqdn := ri.Record.Name
	trimmed := strings.Trim(fqdn, ".")
	parts := strings.Split(trimmed, ".")
	// Reverse parts.
	for i, j := 0, len(parts)-1; i < j; i, j = i+1, j-1 {
		parts[i], parts[j] = parts[j], parts[i]
	}
	baseKey := fmt.Sprintf("%s/%s", er.cfg.PathPrefix, strings.Join(parts, "/"))
	resp, err := er.client.Get(ctx, baseKey, clientv3.WithPrefix())
	if err != nil {
		return err
	}
	for _, kv := range resp.Kvs {
		keyStr := string(kv.Key)
		var wire etcdRecord
		if err := json.Unmarshal(kv.Value, &wire); err != nil {
			er.logger.Warn().Err(err).Msgf("Could not parse key %s", keyStr)
			continue
		}
		// Match based on record fields.
		if wire.Host == ri.Record.Value &&
			wire.RecordType == ri.Record.Type &&
			wire.OwnerHostname == ri.Hostname &&
			wire.OwnerContainerName == ri.ContainerName {
			_, err := er.client.Delete(ctx, keyStr)
			if err != nil {
				er.logger.Warn().Err(err).Msgf("Failed to delete key %s", keyStr)
				return err
			}
			er.logger.Info().Msgf("Deleted key %s", keyStr)
			return nil
		}
	}
	return nil
}

// List retrieves all record intents stored in etcd under the configured prefix.
func (er *EtcdRegistry) List(ctx context.Context) ([]*domain.RecordIntent, error) {
	prefix := er.cfg.PathPrefix
	resp, err := er.client.Get(ctx, prefix, clientv3.WithPrefix())
	if err != nil {
		return nil, err
	}
	var intents []*domain.RecordIntent
	for _, kv := range resp.Kvs {
		keyStr := string(kv.Key)
		ri, err := unmarshalEtcdValue(keyStr, string(kv.Value), er.cfg.PathPrefix)
		if err != nil {
			er.logger.Error().Err(err).Msgf("Failed to parse key: %s", keyStr)
			continue
		}
		intents = append(intents, ri)
	}
	return intents, nil
}

// LockTransaction provides a distributed lock using etcd transactions.
// It takes keys (as a slice of string), tries to acquire locks on all of them,
// runs the function, and finally releases all locks.
func (er *EtcdRegistry) LockTransaction(ctx context.Context, keys []string, fn func() error) error {
	// Ensure unique sorted keys.
	uniqueKeys := keys
	leases := make([]struct {
		lockKey string
		lease   clientv3.LeaseID
	}, 0)

	for _, key := range uniqueKeys {
		lockKey := fmt.Sprintf("/locks/%s", key)
		leaseResp, err := er.client.Grant(ctx, int64(er.cfg.LockTTL))
		if err != nil {
			return fmt.Errorf("failed to create lease: %w", err)
		}
		acquired := false
		start := time.Now()
		for time.Since(start) < time.Duration(er.cfg.LockTimeout)*time.Second {
			txnResp, err := er.client.Txn(ctx).
				If(clientv3.Compare(clientv3.CreateRevision(lockKey), "=", 0)).
				Then(clientv3.OpPut(lockKey, er.hostname, clientv3.WithLease(leaseResp.ID))).
				Commit()
			if err != nil {
				return err
			}
			if txnResp.Succeeded {
				acquired = true
				leases = append(leases, struct {
					lockKey string
					lease   clientv3.LeaseID
				}{lockKey, leaseResp.ID})
				break
			}
			time.Sleep(time.Duration(er.cfg.LockRetryInterval * float64(time.Second)))
		}
		if !acquired {
			return fmt.Errorf("failed to acquire lock on %s", key)
		}
	}

	// Execute the provided function with locks held.
	err := fn()

	// Release the locks in reverse order.
	for i := len(leases) - 1; i >= 0; i-- {
		l := leases[i]
		_, errDel := er.client.Delete(ctx, l.lockKey)
		if errDel != nil {
			er.logger.Warn().Err(errDel).Msgf("failed to delete lock key %s", l.lockKey)
		}
		_, errRevoke := er.client.Revoke(ctx, l.lease)
		if errRevoke != nil {
			er.logger.Warn().Err(errRevoke).Msgf("failed to revoke lease for %s", l.lockKey)
		}
	}
	return err
}

func (er *EtcdRegistry) Close() error {
	return er.client.Close()
}
