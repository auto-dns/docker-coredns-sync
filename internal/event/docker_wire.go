package event

import (
	"strings"
	"time"

	"github.com/auto-dns/docker-coredns-sync/internal/domain"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/events"
)

func fromContainerSummary(c container.Summary) domain.ContainerEvent {
	name := ""
	if len(c.Names) > 0 {
		name = strings.TrimPrefix(c.Names[0], "/")
	}
	return domain.ContainerEvent{
		Container: domain.Container{
			Id:      c.ID,
			Name:    name,
			Created: time.Unix(c.Created, 0),
			Labels:  c.Labels,
		},
		EventType: domain.EventTypeInitialContainerDetection,
	}
}

func fromEventsMessage(msg events.Message) (domain.ContainerEvent, error) {
	ev := domain.ContainerEvent{
		Container: domain.Container{
			Id:      msg.ID,
			Name:    msg.Actor.Attributes["name"],
			Created: time.Unix(0, msg.TimeNano),
			Labels:  msg.Actor.Attributes,
		},
		EventType: domain.EventType(msg.Status),
	}
	if !ev.EventType.IsValid() {
		return domain.ContainerEvent{}, NewUnsupportedEventTypeError(ev.EventType)
	}
	return ev, nil
}
