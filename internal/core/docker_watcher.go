package core

import (
	"context"
)

// DockerWatcher is responsible for emitting ContainerEvents.
type DockerWatcher interface {
	// Subscribe starts watching for events and returns a read-only channel.
	Subscribe(ctx context.Context) (<-chan ContainerEvent, error)
	ListRunningContainers(ctx context.Context) ([]ContainerEvent, error)
	Stop() // used for cleanup
}
