package domain

import "time"

type EventType string

const (
	EventTypeContainerDied             EventType = "die"
	EventTypeContainerStarted          EventType = "start"
	EventTypeContainerStopped          EventType = "stop"
	EventTypeInitialContainerDetection EventType = "initial_detection"
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
		EventTypeInitialContainerDetection:
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
}
