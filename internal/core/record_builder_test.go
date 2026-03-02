package core

import (
	"testing"
	"time"

	"github.com/auto-dns/docker-coredns-sync/internal/config"
	"github.com/auto-dns/docker-coredns-sync/internal/domain"
	"github.com/rs/zerolog"
)

func makeTestConfig() *config.AppConfig {
	return &config.AppConfig{
		DockerLabelPrefix: "coredns",
		HostIPv4:          "10.0.0.1",
		HostIPv6:          "::1",
		Hostname:          "test-host",
		PollInterval:      5,
	}
}

func makeTestConfigNoDefaults() *config.AppConfig {
	return &config.AppConfig{
		DockerLabelPrefix: "coredns",
		HostIPv4:          "",
		HostIPv6:          "",
		Hostname:          "test-host",
		PollInterval:      5,
	}
}

func makeContainerEvent(labels map[string]string) domain.ContainerEvent {
	return domain.ContainerEvent{
		Container: domain.Container{
			Id:      "container-123",
			Name:    "test-container",
			Created: time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC),
			Labels:  labels,
		},
		EventType: domain.EventTypeContainerStarted,
	}
}

func nopLogger() zerolog.Logger {
	return zerolog.Nop()
}

func TestGetContainerRecordIntents_NotEnabled(t *testing.T) {
	cfg := makeTestConfig()
	event := makeContainerEvent(map[string]string{
		"coredns.A.name":  "app.example.com",
		"coredns.A.value": "192.168.1.1",
	})

	intents, err := GetContainerRecordIntents(event, cfg, nopLogger())

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(intents) != 0 {
		t.Errorf("expected 0 intents when not enabled, got %d", len(intents))
	}
}

func TestGetContainerRecordIntents_AWithValue(t *testing.T) {
	cfg := makeTestConfig()
	event := makeContainerEvent(map[string]string{
		"coredns.enabled": "true",
		"coredns.A.name":  "app.example.com",
		"coredns.A.value": "192.168.1.1",
	})

	intents, err := GetContainerRecordIntents(event, cfg, nopLogger())

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(intents) != 1 {
		t.Fatalf("expected 1 intent, got %d", len(intents))
	}

	intent := intents[0]
	if intent.Record.Kind != domain.RecordA {
		t.Errorf("expected RecordA, got %v", intent.Record.Kind)
	}
	if intent.Record.Name != "app.example.com" {
		t.Errorf("expected name 'app.example.com', got %q", intent.Record.Name)
	}
	if intent.Record.Value != "192.168.1.1" {
		t.Errorf("expected value '192.168.1.1', got %q", intent.Record.Value)
	}
}

func TestGetContainerRecordIntents_AWithDefaultIPv4(t *testing.T) {
	cfg := makeTestConfig()
	event := makeContainerEvent(map[string]string{
		"coredns.enabled": "true",
		"coredns.A.name":  "app.example.com",
	})

	intents, err := GetContainerRecordIntents(event, cfg, nopLogger())

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(intents) != 1 {
		t.Fatalf("expected 1 intent, got %d", len(intents))
	}

	intent := intents[0]
	if intent.Record.Value != "10.0.0.1" {
		t.Errorf("expected default value '10.0.0.1', got %q", intent.Record.Value)
	}
}

func TestGetContainerRecordIntents_ANoDefaultSkipped(t *testing.T) {
	cfg := makeTestConfigNoDefaults()
	event := makeContainerEvent(map[string]string{
		"coredns.enabled": "true",
		"coredns.A.name":  "app.example.com",
	})

	intents, err := GetContainerRecordIntents(event, cfg, nopLogger())

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(intents) != 0 {
		t.Errorf("expected 0 intents when no default IP, got %d", len(intents))
	}
}

func TestGetContainerRecordIntents_AAAAWithValue(t *testing.T) {
	cfg := makeTestConfig()
	event := makeContainerEvent(map[string]string{
		"coredns.enabled":    "true",
		"coredns.AAAA.name":  "app.example.com",
		"coredns.AAAA.value": "2001:db8::1",
	})

	intents, err := GetContainerRecordIntents(event, cfg, nopLogger())

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(intents) != 1 {
		t.Fatalf("expected 1 intent, got %d", len(intents))
	}

	intent := intents[0]
	if intent.Record.Kind != domain.RecordAAAA {
		t.Errorf("expected RecordAAAA, got %v", intent.Record.Kind)
	}
	if intent.Record.Value != "2001:db8::1" {
		t.Errorf("expected value '2001:db8::1', got %q", intent.Record.Value)
	}
}

func TestGetContainerRecordIntents_AAAAWithDefaultIPv6(t *testing.T) {
	cfg := makeTestConfig()
	event := makeContainerEvent(map[string]string{
		"coredns.enabled":   "true",
		"coredns.AAAA.name": "app.example.com",
	})

	intents, err := GetContainerRecordIntents(event, cfg, nopLogger())

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(intents) != 1 {
		t.Fatalf("expected 1 intent, got %d", len(intents))
	}

	intent := intents[0]
	if intent.Record.Value != "::1" {
		t.Errorf("expected default value '::1', got %q", intent.Record.Value)
	}
}

func TestGetContainerRecordIntents_AAAANoDefaultSkipped(t *testing.T) {
	cfg := makeTestConfigNoDefaults()
	event := makeContainerEvent(map[string]string{
		"coredns.enabled":   "true",
		"coredns.AAAA.name": "app.example.com",
	})

	intents, err := GetContainerRecordIntents(event, cfg, nopLogger())

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(intents) != 0 {
		t.Errorf("expected 0 intents when no default IPv6, got %d", len(intents))
	}
}

func TestGetContainerRecordIntents_CNAMERequiresValue(t *testing.T) {
	cfg := makeTestConfig()
	event := makeContainerEvent(map[string]string{
		"coredns.enabled":    "true",
		"coredns.CNAME.name": "alias.example.com",
	})

	intents, err := GetContainerRecordIntents(event, cfg, nopLogger())

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(intents) != 0 {
		t.Errorf("expected 0 intents for CNAME without value, got %d", len(intents))
	}
}

func TestGetContainerRecordIntents_CNAMEWithValue(t *testing.T) {
	cfg := makeTestConfig()
	event := makeContainerEvent(map[string]string{
		"coredns.enabled":     "true",
		"coredns.CNAME.name":  "alias.example.com",
		"coredns.CNAME.value": "target.example.com",
	})

	intents, err := GetContainerRecordIntents(event, cfg, nopLogger())

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(intents) != 1 {
		t.Fatalf("expected 1 intent, got %d", len(intents))
	}

	intent := intents[0]
	if intent.Record.Kind != domain.RecordCNAME {
		t.Errorf("expected RecordCNAME, got %v", intent.Record.Kind)
	}
}

func TestGetContainerRecordIntents_SkipsEmptyName(t *testing.T) {
	cfg := makeTestConfig()
	event := makeContainerEvent(map[string]string{
		"coredns.enabled": "true",
		"coredns.A.name":  "",
		"coredns.A.value": "192.168.1.1",
	})

	intents, err := GetContainerRecordIntents(event, cfg, nopLogger())

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(intents) != 0 {
		t.Errorf("expected 0 intents for empty name, got %d", len(intents))
	}
}

func TestGetContainerRecordIntents_SkipsWhitespaceOnlyName(t *testing.T) {
	cfg := makeTestConfig()
	event := makeContainerEvent(map[string]string{
		"coredns.enabled": "true",
		"coredns.A.name":  "   ",
		"coredns.A.value": "192.168.1.1",
	})

	intents, err := GetContainerRecordIntents(event, cfg, nopLogger())

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(intents) != 0 {
		t.Errorf("expected 0 intents for whitespace-only name, got %d", len(intents))
	}
}

func TestGetContainerRecordIntents_InvalidRecordSkipped(t *testing.T) {
	cfg := makeTestConfig()
	event := makeContainerEvent(map[string]string{
		"coredns.enabled": "true",
		"coredns.A.name":  "valid.example.com",
		"coredns.A.value": "not-an-ip", // Invalid IP
	})

	intents, err := GetContainerRecordIntents(event, cfg, nopLogger())

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(intents) != 0 {
		t.Errorf("expected 0 intents for invalid record, got %d", len(intents))
	}
}

func TestGetContainerRecordIntents_ForceInheritedFromContainer(t *testing.T) {
	cfg := makeTestConfig()
	event := makeContainerEvent(map[string]string{
		"coredns.enabled": "true",
		"coredns.force":   "true",
		"coredns.A.name":  "app.example.com",
		"coredns.A.value": "192.168.1.1",
	})

	intents, err := GetContainerRecordIntents(event, cfg, nopLogger())

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(intents) != 1 {
		t.Fatalf("expected 1 intent, got %d", len(intents))
	}

	if !intents[0].Force {
		t.Error("expected Force to be true (inherited from container)")
	}
}

func TestGetContainerRecordIntents_ForceOverriddenPerRecord(t *testing.T) {
	cfg := makeTestConfig()
	event := makeContainerEvent(map[string]string{
		"coredns.enabled":       "true",
		"coredns.force":         "true",
		"coredns.A.web.name":    "web.example.com",
		"coredns.A.web.value":   "192.168.1.1",
		"coredns.A.web.force":   "false", // Override container force
		"coredns.A.api.name":    "api.example.com",
		"coredns.A.api.value":   "192.168.1.2",
	})

	intents, err := GetContainerRecordIntents(event, cfg, nopLogger())

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(intents) != 2 {
		t.Fatalf("expected 2 intents, got %d", len(intents))
	}

	for _, intent := range intents {
		switch intent.Record.Name {
		case "web.example.com":
			if intent.Force {
				t.Error("expected web Force to be false (overridden)")
			}
		case "api.example.com":
			if !intent.Force {
				t.Error("expected api Force to be true (inherited)")
			}
		}
	}
}

func TestGetContainerRecordIntents_SetsContainerMetadata(t *testing.T) {
	cfg := makeTestConfig()
	created := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
	event := domain.ContainerEvent{
		Container: domain.Container{
			Id:      "abc123",
			Name:    "my-container",
			Created: created,
			Labels: map[string]string{
				"coredns.enabled": "true",
				"coredns.A.name":  "app.example.com",
				"coredns.A.value": "192.168.1.1",
			},
		},
		EventType: domain.EventTypeContainerStarted,
	}

	intents, err := GetContainerRecordIntents(event, cfg, nopLogger())

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(intents) != 1 {
		t.Fatalf("expected 1 intent, got %d", len(intents))
	}

	intent := intents[0]
	if intent.ContainerId != "abc123" {
		t.Errorf("expected ContainerId 'abc123', got %q", intent.ContainerId)
	}
	if intent.ContainerName != "my-container" {
		t.Errorf("expected ContainerName 'my-container', got %q", intent.ContainerName)
	}
	if !intent.Created.Equal(created) {
		t.Errorf("expected Created %v, got %v", created, intent.Created)
	}
	if intent.Hostname != "test-host" {
		t.Errorf("expected Hostname 'test-host', got %q", intent.Hostname)
	}
}

func TestGetContainerRecordIntents_MultipleRecords(t *testing.T) {
	cfg := makeTestConfig()
	event := makeContainerEvent(map[string]string{
		"coredns.enabled":     "true",
		"coredns.A.name":      "app.example.com",
		"coredns.A.value":     "192.168.1.1",
		"coredns.AAAA.name":   "app.example.com",
		"coredns.AAAA.value":  "::1",
		"coredns.CNAME.name":  "alias.example.com",
		"coredns.CNAME.value": "app.example.com",
	})

	intents, err := GetContainerRecordIntents(event, cfg, nopLogger())

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(intents) != 3 {
		t.Fatalf("expected 3 intents, got %d", len(intents))
	}
}

func TestGetContainerRecordIntents_MultipleAliases(t *testing.T) {
	cfg := makeTestConfig()
	event := makeContainerEvent(map[string]string{
		"coredns.enabled":     "true",
		"coredns.A.web.name":  "web.example.com",
		"coredns.A.web.value": "192.168.1.1",
		"coredns.A.api.name":  "api.example.com",
		"coredns.A.api.value": "192.168.1.2",
		"coredns.A.db.name":   "db.example.com",
		"coredns.A.db.value":  "192.168.1.3",
	})

	intents, err := GetContainerRecordIntents(event, cfg, nopLogger())

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(intents) != 3 {
		t.Fatalf("expected 3 intents, got %d", len(intents))
	}

	names := make(map[string]bool)
	for _, intent := range intents {
		names[intent.Record.Name] = true
	}

	expected := []string{"web.example.com", "api.example.com", "db.example.com"}
	for _, name := range expected {
		if !names[name] {
			t.Errorf("expected to find intent for %q", name)
		}
	}
}

func TestGetContainerRecordIntents_ValidAndInvalidMixed(t *testing.T) {
	cfg := makeTestConfig()
	event := makeContainerEvent(map[string]string{
		"coredns.enabled":       "true",
		"coredns.A.valid.name":  "valid.example.com",
		"coredns.A.valid.value": "192.168.1.1",
		"coredns.A.bad.name":    "bad.example.com",
		"coredns.A.bad.value":   "not-an-ip", // Invalid
		"coredns.A.good.name":   "good.example.com",
		"coredns.A.good.value":  "192.168.1.2",
	})

	intents, err := GetContainerRecordIntents(event, cfg, nopLogger())

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(intents) != 2 {
		t.Fatalf("expected 2 valid intents, got %d", len(intents))
	}
}
