package event

import (
	"testing"
	"time"

	"github.com/auto-dns/docker-coredns-sync/internal/domain"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/events"
)

func TestFromContainerSummary_BasicFields(t *testing.T) {
	summary := container.Summary{
		ID:      "abc123def456",
		Names:   []string{"/my-container"},
		Created: 1704067200, // 2024-01-01 00:00:00 UTC
		Labels: map[string]string{
			"coredns.enabled": "true",
			"coredns.a.app":   "192.168.1.1",
		},
	}

	result := fromContainerSummary(summary)

	if result.Container.Id != "abc123def456" {
		t.Errorf("expected Id 'abc123def456', got %q", result.Container.Id)
	}
	if result.Container.Name != "my-container" {
		t.Errorf("expected Name 'my-container' (without slash), got %q", result.Container.Name)
	}
	if result.EventType != domain.EventTypeInitialContainerDetection {
		t.Errorf("expected EventType InitialContainerDetection, got %v", result.EventType)
	}
}

func TestFromContainerSummary_StripsLeadingSlash(t *testing.T) {
	tests := []struct {
		names    []string
		expected string
	}{
		{[]string{"/container"}, "container"},
		{[]string{"/my-app"}, "my-app"},
		{[]string{"/deeply/nested"}, "deeply/nested"}, // only first slash
	}

	for _, tt := range tests {
		t.Run(tt.names[0], func(t *testing.T) {
			summary := container.Summary{
				ID:      "test",
				Names:   tt.names,
				Created: 0,
				Labels:  nil,
			}

			result := fromContainerSummary(summary)

			if result.Container.Name != tt.expected {
				t.Errorf("expected Name %q, got %q", tt.expected, result.Container.Name)
			}
		})
	}
}

func TestFromContainerSummary_EmptyNames(t *testing.T) {
	summary := container.Summary{
		ID:      "abc123",
		Names:   []string{},
		Created: 0,
		Labels:  nil,
	}

	result := fromContainerSummary(summary)

	if result.Container.Name != "" {
		t.Errorf("expected empty Name for empty Names, got %q", result.Container.Name)
	}
}

func TestFromContainerSummary_MultipleNames(t *testing.T) {
	summary := container.Summary{
		ID:      "abc123",
		Names:   []string{"/primary", "/secondary", "/tertiary"},
		Created: 0,
		Labels:  nil,
	}

	result := fromContainerSummary(summary)

	// Should use the first name
	if result.Container.Name != "primary" {
		t.Errorf("expected 'primary' (first name), got %q", result.Container.Name)
	}
}

func TestFromContainerSummary_CreatedTime(t *testing.T) {
	timestamp := int64(1704067200) // 2024-01-01 00:00:00 UTC
	summary := container.Summary{
		ID:      "abc123",
		Names:   []string{"/test"},
		Created: timestamp,
		Labels:  nil,
	}

	result := fromContainerSummary(summary)

	expected := time.Unix(timestamp, 0)
	if !result.Container.Created.Equal(expected) {
		t.Errorf("expected Created %v, got %v", expected, result.Container.Created)
	}
}

func TestFromContainerSummary_Labels(t *testing.T) {
	labels := map[string]string{
		"coredns.enabled": "true",
		"coredns.a.web":   "192.168.1.1",
		"other.label":     "value",
	}
	summary := container.Summary{
		ID:      "abc123",
		Names:   []string{"/test"},
		Created: 0,
		Labels:  labels,
	}

	result := fromContainerSummary(summary)

	if len(result.Container.Labels) != 3 {
		t.Errorf("expected 3 labels, got %d", len(result.Container.Labels))
	}
	if result.Container.Labels["coredns.enabled"] != "true" {
		t.Error("expected coredns.enabled label")
	}
}

func TestFromEventsMessage_StartEvent(t *testing.T) {
	msg := events.Message{
		ID:       "abc123def456",
		Status:   "start",
		TimeNano: 1704067200000000000, // 2024-01-01 00:00:00 UTC in nanoseconds
		Actor: events.Actor{
			Attributes: map[string]string{
				"name":            "my-container",
				"coredns.enabled": "true",
			},
		},
	}

	result, err := fromEventsMessage(msg)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Container.Id != "abc123def456" {
		t.Errorf("expected Id 'abc123def456', got %q", result.Container.Id)
	}
	if result.Container.Name != "my-container" {
		t.Errorf("expected Name 'my-container', got %q", result.Container.Name)
	}
	if result.EventType != domain.EventTypeContainerStarted {
		t.Errorf("expected EventType ContainerStarted, got %v", result.EventType)
	}
}

func TestFromEventsMessage_DieEvent(t *testing.T) {
	msg := events.Message{
		ID:       "abc123",
		Status:   "die",
		TimeNano: 0,
		Actor: events.Actor{
			Attributes: map[string]string{
				"name": "dying-container",
			},
		},
	}

	result, err := fromEventsMessage(msg)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.EventType != domain.EventTypeContainerDied {
		t.Errorf("expected EventType ContainerDied, got %v", result.EventType)
	}
}

func TestFromEventsMessage_StopEvent(t *testing.T) {
	msg := events.Message{
		ID:       "abc123",
		Status:   "stop",
		TimeNano: 0,
		Actor: events.Actor{
			Attributes: map[string]string{
				"name": "stopped-container",
			},
		},
	}

	result, err := fromEventsMessage(msg)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.EventType != domain.EventTypeContainerStopped {
		t.Errorf("expected EventType ContainerStopped, got %v", result.EventType)
	}
}

func TestFromEventsMessage_UnsupportedEvent(t *testing.T) {
	unsupportedStatuses := []string{
		"create",
		"attach",
		"commit",
		"copy",
		"exec_create",
		"exec_start",
		"export",
		"health_status",
		"oom",
		"pause",
		"rename",
		"resize",
		"restart",
		"kill",
		"destroy",
		"top",
		"unpause",
		"update",
		"prune",
		"unknown",
	}

	for _, status := range unsupportedStatuses {
		t.Run(status, func(t *testing.T) {
			msg := events.Message{
				ID:       "abc123",
				Status:   status,
				TimeNano: 0,
				Actor: events.Actor{
					Attributes: map[string]string{
						"name": "test",
					},
				},
			}

			_, err := fromEventsMessage(msg)

			if err == nil {
				t.Errorf("expected error for unsupported event status %q", status)
			}

			// Verify it's the right error type
			_, ok := err.(*UnsupportedEventTypeError)
			if !ok {
				t.Errorf("expected UnsupportedEventTypeError, got %T", err)
			}
		})
	}
}

func TestFromEventsMessage_TimeNano(t *testing.T) {
	// Time in nanoseconds: 2024-01-15 10:30:00.123456789 UTC
	timeNano := int64(1705315800123456789)
	msg := events.Message{
		ID:       "abc123",
		Status:   "start",
		TimeNano: timeNano,
		Actor: events.Actor{
			Attributes: map[string]string{
				"name": "test",
			},
		},
	}

	result, err := fromEventsMessage(msg)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := time.Unix(0, timeNano)
	if !result.Container.Created.Equal(expected) {
		t.Errorf("expected Created %v, got %v", expected, result.Container.Created)
	}
}

func TestFromEventsMessage_LabelsFromAttributes(t *testing.T) {
	msg := events.Message{
		ID:       "abc123",
		Status:   "start",
		TimeNano: 0,
		Actor: events.Actor{
			Attributes: map[string]string{
				"name":            "my-container",
				"coredns.enabled": "true",
				"coredns.a.web":   "192.168.1.1",
				"image":           "nginx:latest",
			},
		},
	}

	result, err := fromEventsMessage(msg)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// All attributes become labels
	if len(result.Container.Labels) != 4 {
		t.Errorf("expected 4 labels, got %d", len(result.Container.Labels))
	}
	if result.Container.Labels["coredns.enabled"] != "true" {
		t.Error("expected coredns.enabled label")
	}
	if result.Container.Labels["name"] != "my-container" {
		t.Error("expected name to be in labels as well")
	}
}

func TestFromEventsMessage_EmptyAttributes(t *testing.T) {
	msg := events.Message{
		ID:       "abc123",
		Status:   "start",
		TimeNano: 0,
		Actor: events.Actor{
			Attributes: map[string]string{},
		},
	}

	result, err := fromEventsMessage(msg)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Container.Name != "" {
		t.Errorf("expected empty name, got %q", result.Container.Name)
	}
	if len(result.Container.Labels) != 0 {
		t.Errorf("expected 0 labels, got %d", len(result.Container.Labels))
	}
}

func TestFromEventsMessage_NilAttributes(t *testing.T) {
	msg := events.Message{
		ID:       "abc123",
		Status:   "start",
		TimeNano: 0,
		Actor: events.Actor{
			Attributes: nil,
		},
	}

	result, err := fromEventsMessage(msg)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should handle nil gracefully
	if result.Container.Name != "" {
		t.Errorf("expected empty name, got %q", result.Container.Name)
	}
}
