package registry

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/auto-dns/docker-coredns-sync/internal/config"
	"github.com/auto-dns/docker-coredns-sync/internal/domain"
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
		logger:   logger.With().Str("component", "etcd_registry").Logger(),
	}
}

// getNextIndexedKey generates a new etcd key for a record based on its fully qualified domain name (fqdn).
func (er *EtcdRegistry) getNextIndexedKey(ctx context.Context, fqdn string) (string, error) {
	base := keyBaseForFQDN(er.cfg.PathPrefix, fqdn)
	existing := make(map[int]struct{})

	resp, err := er.client.Get(ctx, base, clientv3.WithPrefix(), clientv3.WithKeysOnly(), clientv3.WithSerializable())
	if err != nil {
		return "", fmt.Errorf("list existing indices under %q: %w", base, err)
	}
	for _, kv := range resp.Kvs {
		keyStr := string(kv.Key)
		// ensure we're only looking at immediate children under base
		if !strings.HasPrefix(keyStr, base+"/") {
			continue
		}

		suffix := keyStr[len(base)+1:] // after trailing slash
		if strings.HasPrefix(suffix, "x") {
			if n, err := strconv.Atoi(suffix[1:]); err == nil {
				existing[n] = struct{}{}
			}
		}
	}
	idx := 1
	for {
		if _, exists := existing[idx]; !exists {
			break
		}
		idx++
	}
	return fmt.Sprintf("%s/x%d", base, idx), nil
}

// Register stores the record intent in etcd.
func (er *EtcdRegistry) Register(ctx context.Context, ri *domain.RecordIntent) error {
	fqdn := ri.Record.Name
	key, err := er.getNextIndexedKey(ctx, fqdn)
	if err != nil {
		return fmt.Errorf("compute next key for %q: %w", fqdn, err)
	}
	value, err := marshalEtcdValue(ri)
	if err != nil {
		return fmt.Errorf("marshal etcd value for %q: %w", fqdn, err)
	}
	if _, err := er.client.Put(ctx, key, value); err != nil {
		return fmt.Errorf("put key %q: %w", key, err)
	}
	er.logger.Info().Str("fqdn", ri.Record.Name).Str("kind", string(ri.Record.Kind)).Str("host", ri.Record.Value).Str("owner_hostname", ri.Hostname).Str("owner_container_id", ri.ContainerId).Str("key", key).Msg("registered record")
	return nil
}

func (er *EtcdRegistry) recordMatches(w etcdRecord, ri *domain.RecordIntent) bool {
	return w.Host == ri.Record.Value &&
		w.Kind == ri.Record.Kind &&
		w.OwnerHostname == ri.Hostname &&
		w.OwnerContainerName == ri.ContainerName &&
		(ri.ContainerId == "" || w.OwnerContainerId == ri.ContainerId)
}

// Remove finds and deletes the etcd key that matches the record domain.
func (er *EtcdRegistry) Remove(ctx context.Context, ri *domain.RecordIntent) error {
	base := keyBaseForFQDN(er.cfg.PathPrefix, ri.Record.Name)

	resp, err := er.client.Get(ctx, base, clientv3.WithPrefix())
	if err != nil {
		return fmt.Errorf("list keys under base %q: %w", base, err)
	}

	// Collect keys to delete
	toDelete := make([]string, 0, len(resp.Kvs))
	for _, kv := range resp.Kvs {
		keyStr := string(kv.Key)
		if !strings.HasPrefix(keyStr, base+"/") {
			continue
		}
		var wire etcdRecord
		if err := json.Unmarshal(kv.Value, &wire); err != nil {
			er.logger.Warn().Err(err).Str("key", keyStr).Msg("unmarshal etcd record")
			continue
		}
		if er.recordMatches(wire, ri) {
			toDelete = append(toDelete, keyStr)
		}
	}

	if len(toDelete) == 0 {
		er.logger.Debug().
			Str("fqdn", ri.Record.Name).Str("kind", string(ri.Record.Kind)).
			Str("host", ri.Record.Value).Str("owner_hostname", ri.Hostname).
			Str("owner_container_name", ri.ContainerName).
			Msg("remove: no matching keys")
		return nil
	}

	// Delete all matches (keep going on errors)
	// delete in batches to avoid overly-large transactions
	const batchSize = 64
	var firstErr error

	for i := 0; i < len(toDelete); i += batchSize {
		end := i + batchSize
		if end > len(toDelete) {
			end = len(toDelete)
		}
		batch := toDelete[i:end]

		txn := er.client.Txn(ctx)
		ops := make([]clientv3.Op, 0, len(batch))
		for _, k := range batch {
			ops = append(ops, clientv3.OpDelete(k))
		}
		if _, err := txn.Then(ops...).Commit(); err != nil {
			er.logger.Warn().Err(err).Int("batch_start", i).Int("batch_end", end).Msg("remove: batch delete failed")
			if firstErr == nil {
				firstErr = fmt.Errorf("batch delete [%d:%d]: %w", i, end, err)
			}
		} else {
			for _, k := range batch {
				er.logger.Info().Str("key", k).Str("fqdn", ri.Record.Name).Str("kind", string(ri.Record.Kind)).Str("host", ri.Record.Value).Str("owner_hostname", ri.Hostname).Str("owner_container_id", ri.ContainerId).Msg("remove: deleted record")
			}
		}
	}

	return firstErr
}

// List retrieves all record intents stored in etcd under the configured prefix.
func (er *EtcdRegistry) List(ctx context.Context) ([]*domain.RecordIntent, error) {
	prefix := er.cfg.PathPrefix
	resp, err := er.client.Get(ctx, prefix, clientv3.WithPrefix(), clientv3.WithSerializable())
	if err != nil {
		return nil, fmt.Errorf("list under prefix %q: %w", prefix, err)
	}
	intents := make([]*domain.RecordIntent, 0, len(resp.Kvs))
	for _, kv := range resp.Kvs {
		ri, err := unmarshalEtcdValue(string(kv.Key), string(kv.Value), er.cfg.PathPrefix)
		if err != nil {
			er.logger.Warn().Err(err).Msgf("Failed to parse key: %s", kv.Key)
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
	uniq := make(map[string]struct{}, len(keys))
	for _, k := range keys {
		uniq[k] = struct{}{}
	}
	uniqueKeys := make([]string, 0, len(uniq))
	for k := range uniq {
		uniqueKeys = append(uniqueKeys, k)
	}
	sort.Strings(uniqueKeys)

	leases := make([]heldLease, 0, len(uniqueKeys))

	for _, key := range uniqueKeys {
		lockKey := fmt.Sprintf("/locks/%s", key)
		leaseResp, err := er.client.Grant(ctx, int64(er.cfg.LockTTL))
		if err != nil {
			return fmt.Errorf("failed to create lease: %w", err)
		}
		acquired := false
		start := time.Now()
		for time.Since(start) < time.Duration(er.cfg.LockTimeout)*time.Second {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}

			txnResp, err := er.client.Txn(ctx).
				If(clientv3.Compare(clientv3.CreateRevision(lockKey), "=", 0)).
				Then(clientv3.OpPut(lockKey, er.hostname, clientv3.WithLease(leaseResp.ID))).
				Commit()
			if err != nil {
				return fmt.Errorf("acquire lock %q: %w", lockKey, err)
			}
			if txnResp.Succeeded {
				acquired = true

				// Start keepalive for this lease
				kaCtx, cancel := context.WithCancel(ctx)
				kaCh, err := er.client.KeepAlive(kaCtx, leaseResp.ID)
				if err != nil {
					cancel()
					// best-effort cleanup
					_, _ = er.client.Delete(ctx, lockKey)
					_, _ = er.client.Revoke(ctx, leaseResp.ID)
					return fmt.Errorf("keepalive for lock %q: %w", lockKey, err)
				}

				// Drain keepalive responses until cancel
				go func() {
					for range kaCh {
						// noop; presence keeps the lease alive
					}
				}()

				leases = append(leases, heldLease{lockKey: lockKey, lease: leaseResp.ID, cancel: cancel})
				break
			}
			time.Sleep(time.Duration(er.cfg.LockRetryInterval * float64(time.Second)))
		}
		if !acquired {
			if _, e := er.client.Revoke(ctx, leaseResp.ID); e != nil {
				er.logger.Warn().Err(e).Str("lock", lockKey).Msg("revoke unused lease after acquire timeout")
			}
			return fmt.Errorf("failed to acquire lock on %s", key)
		}
	}

	err := fn()

	// Release the locks in reverse order
	for i := len(leases) - 1; i >= 0; i-- {
		l := leases[i]
		l.cancel() // Stop the keepalive first
		if _, e := er.client.Delete(ctx, l.lockKey); e != nil {
			er.logger.Warn().Err(e).Msgf("failed to delete lock key %s", l.lockKey)
		}
		if _, e := er.client.Revoke(ctx, l.lease); e != nil {
			er.logger.Warn().Err(e).Msgf("failed to revoke lease for %s", l.lockKey)
		}
	}
	return err
}

func (er *EtcdRegistry) Close() error {
	return er.client.Close()
}
