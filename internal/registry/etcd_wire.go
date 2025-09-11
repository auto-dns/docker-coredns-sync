package registry

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/auto-dns/docker-coredns-sync/internal/domain"
)

type etcdRecord struct {
	Host               string            `json:"host"`
	RecordType         domain.RecordKind `json:"record_type"`
	OwnerHostname      string            `json:"owner_hostname"`
	OwnerContainerId   string            `json:"owner_container_id"`
	OwnerContainerName string            `json:"owner_container_name"`
	Created            time.Time         `json:"created"`
	Force              bool              `json:"force"`
}

func marshalEtcdValue(ri *domain.RecordIntent) (string, error) {
	wire := etcdRecord{
		Host:               ri.Record.Value,
		RecordType:         ri.Record.Type,
		OwnerHostname:      ri.Hostname,
		OwnerContainerId:   ri.ContainerId,
		OwnerContainerName: ri.ContainerName,
		Created:            ri.Created,
		Force:              ri.Force,
	}
	b, err := json.Marshal(wire)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func unmarshalEtcdValue(key string, raw string, prefix string) (*domain.RecordIntent, error) {
	fqdn := fqdnFromKey(prefix, key)

	var wire etcdRecord
	if err := json.Unmarshal([]byte(raw), &wire); err != nil {
		return nil, fmt.Errorf("decode etcd value: %w", err)
	}

	rec, err := domain.NewFromKind(wire.RecordType, fqdn, wire.Host)
	if err != nil {
		return nil, err
	}

	return &domain.RecordIntent{
		ContainerId:   wire.OwnerContainerId,
		ContainerName: wire.OwnerContainerName,
		Created:       wire.Created,
		Hostname:      wire.OwnerHostname,
		Force:         wire.Force,
		Record:        rec,
	}, nil
}
