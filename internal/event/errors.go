package event

import (
	"fmt"

	"github.com/auto-dns/docker-coredns-sync/internal/domain"
)

type UnsupportedEventTypeError struct {
	eventType domain.EventType
}

func NewUnsupportedEventTypeError(eventType domain.EventType) *UnsupportedEventTypeError {
	return &UnsupportedEventTypeError{eventType: eventType}
}

func (e *UnsupportedEventTypeError) Error() string {
	return fmt.Sprintf("Unsupported event type: %s", e.eventType)
}
