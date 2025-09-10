package domain

import "time"

type EventType string

const (
	EventTypeContainerDied             = "die"
	EventTypeContainerStarted          = "start"
	EventTypeContainerStopped          = "stop"
	EventTypeInitialContainerDetection = "inital_detection"
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
