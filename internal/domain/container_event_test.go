package domain

import (
	"testing"
)

func TestEventType_IsValid_ValidTypes(t *testing.T) {
	validTypes := []EventType{
		EventTypeContainerDied,
		EventTypeContainerStarted,
		EventTypeContainerStopped,
		EventTypeInitialContainerDetection,
	}

	for _, et := range validTypes {
		t.Run(string(et), func(t *testing.T) {
			if !et.IsValid() {
				t.Errorf("expected %q to be valid", et)
			}
		})
	}
}

func TestEventType_IsValid_InvalidTypes(t *testing.T) {
	invalidTypes := []EventType{
		"create",
		"destroy",
		"pause",
		"unpause",
		"kill",
		"restart",
		"unknown",
		"",
		"START", // case sensitive
		"STOP",
	}

	for _, et := range invalidTypes {
		t.Run(string(et), func(t *testing.T) {
			if et.IsValid() {
				t.Errorf("expected %q to be invalid", et)
			}
		})
	}
}

func TestEventType_Constants(t *testing.T) {
	// Verify the constant values match expected Docker event types
	if EventTypeContainerDied != "die" {
		t.Errorf("expected EventTypeContainerDied to be 'die', got %q", EventTypeContainerDied)
	}
	if EventTypeContainerStarted != "start" {
		t.Errorf("expected EventTypeContainerStarted to be 'start', got %q", EventTypeContainerStarted)
	}
	if EventTypeContainerStopped != "stop" {
		t.Errorf("expected EventTypeContainerStopped to be 'stop', got %q", EventTypeContainerStopped)
	}
	if EventTypeInitialContainerDetection != "initial_detection" {
		t.Errorf("expected EventTypeInitialContainerDetection to be 'initial_detection', got %q", EventTypeInitialContainerDetection)
	}
}

func TestContainer_FieldAccess(t *testing.T) {
	labels := map[string]string{
		"coredns.enabled": "true",
		"coredns.A.name":  "app.example.com",
	}

	c := Container{
		Id:     "abc123def456",
		Name:   "my-container",
		Labels: labels,
	}

	if c.Id != "abc123def456" {
		t.Errorf("expected Id 'abc123def456', got %q", c.Id)
	}
	if c.Name != "my-container" {
		t.Errorf("expected Name 'my-container', got %q", c.Name)
	}
	if len(c.Labels) != 2 {
		t.Errorf("expected 2 labels, got %d", len(c.Labels))
	}
	if c.Labels["coredns.enabled"] != "true" {
		t.Errorf("expected label coredns.enabled='true', got %q", c.Labels["coredns.enabled"])
	}
}

func TestContainer_EmptyLabels(t *testing.T) {
	c := Container{
		Id:     "abc123",
		Name:   "test",
		Labels: nil,
	}

	// Should not panic when Labels is nil
	if c.Labels != nil {
		t.Error("expected Labels to be nil")
	}
}

func TestContainerEvent_FieldAccess(t *testing.T) {
	c := Container{
		Id:     "abc123",
		Name:   "my-app",
		Labels: map[string]string{"key": "value"},
	}

	evt := ContainerEvent{
		Container: c,
		EventType: EventTypeContainerStarted,
	}

	if evt.Container.Id != "abc123" {
		t.Errorf("expected Container.Id 'abc123', got %q", evt.Container.Id)
	}
	if evt.EventType != EventTypeContainerStarted {
		t.Errorf("expected EventType 'start', got %q", evt.EventType)
	}
}

func TestContainerEvent_AllEventTypes(t *testing.T) {
	c := Container{Id: "test", Name: "test"}

	eventTypes := []EventType{
		EventTypeContainerDied,
		EventTypeContainerStarted,
		EventTypeContainerStopped,
		EventTypeInitialContainerDetection,
	}

	for _, et := range eventTypes {
		t.Run(string(et), func(t *testing.T) {
			evt := ContainerEvent{
				Container: c,
				EventType: et,
			}

			if evt.EventType != et {
				t.Errorf("expected EventType %q, got %q", et, evt.EventType)
			}
			if !evt.EventType.IsValid() {
				t.Errorf("expected EventType %q to be valid", et)
			}
		})
	}
}
