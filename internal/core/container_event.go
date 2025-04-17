package core

import "time"

// ContainerEvent represents a simplified Docker container event.
type ContainerEvent struct {
	ID      string
	Name    string
	Status  string    // e.g. "start", "die", etc.
	Created time.Time // when the container was created
	Labels  map[string]string
}
