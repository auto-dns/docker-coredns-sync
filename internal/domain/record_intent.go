package domain

import (
	"fmt"
	"time"
)

type RecordIntent struct {
	ContainerId   string
	ContainerName string
	Created       time.Time
	Hostname      string
	Force         bool
	// TTL is the DNS record TTL in seconds. Zero means "unset" — the field is
	// omitted from the etcd value and CoreDNS applies its own default.
	TTL    uint32
	Record Record
}

func (ri RecordIntent) Render() string {
	return fmt.Sprintf("%s (container_id=%s, container_name=%s, hostname=%s, created=%s, force=%t, ttl=%d)", ri.Record.Render(), ri.ContainerId, ri.ContainerName, ri.Hostname, ri.Created.Format("2006-01-02 15:04:05"), ri.Force, ri.TTL)
}

func (ri RecordIntent) Equal(other RecordIntent) bool {
	return ri.ContainerId == other.ContainerId &&
		ri.ContainerName == other.ContainerName &&
		ri.Hostname == other.Hostname &&
		ri.Force == other.Force &&
		ri.TTL == other.TTL &&
		ri.Record.Equal(other.Record)
}

func (ri RecordIntent) Key() string {
	return fmt.Sprintf("%s|%s|%s|%t|%d|%s", ri.ContainerId, ri.ContainerName, ri.Hostname, ri.Force, ri.TTL, ri.Record.Key())
}
