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
	Record        Record
}

func (ri RecordIntent) Render() string {
	return fmt.Sprintf("%s (container_id=%s, container_name=%s, hostname=%s, created=%s, force=%t)", ri.Record.Render(), ri.ContainerId, ri.ContainerName, ri.Hostname, ri.Created.Format("2006-01-02 15:04:05"), ri.Force)
}

func (ri RecordIntent) Equal(other RecordIntent) bool {
	return ri.ContainerId == other.ContainerId &&
		ri.ContainerName == other.ContainerName &&
		ri.Hostname == other.Hostname &&
		ri.Force == other.Force &&
		ri.Record.Equal(other.Record)
}

func (ri RecordIntent) Key() string {
	return fmt.Sprintf("%s|%s|%s|%t|%s", ri.ContainerId, ri.ContainerName, ri.Hostname, ri.Force, ri.Record.Key())
}
