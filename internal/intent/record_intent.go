package intent

import (
	"time"

	"github.com/StevenC4/docker-coredns-sync/internal/dns"
)

type RecordIntent struct {
	ContainerID   string
	ContainerName string
	Created       time.Time
	Hostname      string
	Force         bool
	Record        dns.Record
}

func (ri RecordIntent) Equal(other RecordIntent) bool {
	return ri.ContainerID == other.ContainerID &&
		ri.ContainerName == other.ContainerName &&
		ri.Hostname == other.Hostname &&
		ri.Force == other.Force &&
		ri.Record.Equal(other.Record)
}
