package domain

import "time"

type EventType string

const (
	EventTypeContainerDied             EventType = "die"
	EventTypeContainerStarted          EventType = "start"
	EventTypeContainerStopped          EventType = "stop"
	EventTypeInitialContainerDetection EventType = "initial_detection"
	// EventTypeResync carries the full set of currently-running container IDs
	// observed on a (re)connection to the Docker daemon, so the engine can
	// prune state for containers that disappeared while it was disconnected.
	EventTypeResync EventType = "resync"
)

// ContainerStatus is the in-memory lifecycle state the tracker keeps per container.
type ContainerStatus string

const (
	StatusRunning ContainerStatus = "running"
	StatusRemoved ContainerStatus = "removed"
)

func (et EventType) IsValid() bool {
	switch et {
	case EventTypeContainerDied,
		EventTypeContainerStarted,
		EventTypeContainerStopped,
		EventTypeInitialContainerDetection,
		EventTypeResync:
		return true
	}
	return false
}

type Container struct {
	Id      string
	Name    string
	Created time.Time // when the container was created
	Labels  map[string]string
}

type ContainerEvent struct {
	Container Container
	EventType EventType
	// RunningContainerIds is set only for EventTypeResync events: the IDs of
	// all containers running at the moment of (re)connection.
	RunningContainerIds []string
}
