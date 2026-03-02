package event

import (
	"strings"
	"testing"

	"github.com/auto-dns/docker-coredns-sync/internal/domain"
)

func TestNewUnsupportedEventTypeError(t *testing.T) {
	eventType := domain.EventType("unknown_event")

	err := NewUnsupportedEventTypeError(eventType)

	if err == nil {
		t.Fatal("expected non-nil error")
	}
	if err.eventType != eventType {
		t.Errorf("expected eventType %q, got %q", eventType, err.eventType)
	}
}

func TestUnsupportedEventTypeError_Error(t *testing.T) {
	tests := []struct {
		eventType domain.EventType
		expected  string
	}{
		{"unknown", "Unsupported event type: unknown"},
		{"pause", "Unsupported event type: pause"},
		{"restart", "Unsupported event type: restart"},
		{"stop", "Unsupported event type: stop"},
	}

	for _, tt := range tests {
		t.Run(string(tt.eventType), func(t *testing.T) {
			err := NewUnsupportedEventTypeError(tt.eventType)

			result := err.Error()

			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestUnsupportedEventTypeError_ImplementsError(t *testing.T) {
	var err error = NewUnsupportedEventTypeError("test")

	if err == nil {
		t.Fatal("expected non-nil error interface")
	}

	// Should be able to use Error() method
	msg := err.Error()
	if !strings.Contains(msg, "test") {
		t.Errorf("expected error message to contain 'test', got %q", msg)
	}
}

func TestUnsupportedEventTypeError_ContainsEventType(t *testing.T) {
	eventType := domain.EventType("my_custom_event")
	err := NewUnsupportedEventTypeError(eventType)

	msg := err.Error()

	if !strings.Contains(msg, "my_custom_event") {
		t.Errorf("expected error message to contain event type, got %q", msg)
	}
}

func TestUnsupportedEventTypeError_EmptyEventType(t *testing.T) {
	err := NewUnsupportedEventTypeError("")

	msg := err.Error()

	// Should still work with empty event type
	if !strings.Contains(msg, "Unsupported event type") {
		t.Errorf("expected error message format, got %q", msg)
	}
}
