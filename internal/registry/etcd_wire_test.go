package registry

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/auto-dns/docker-coredns-sync/internal/domain"
)

func TestMarshalEtcdValue_AllFields(t *testing.T) {
	created := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	rec, _ := domain.NewA("app.example.com", "192.168.1.1")
	ri := &domain.RecordIntent{
		ContainerId:   "abc123",
		ContainerName: "my-app",
		Created:       created,
		Hostname:      "docker-host",
		Force:         true,
		Record:        rec,
	}

	result, err := marshalEtcdValue(ri)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify it's valid JSON
	var wire etcdRecord
	if err := json.Unmarshal([]byte(result), &wire); err != nil {
		t.Fatalf("result is not valid JSON: %v", err)
	}

	if wire.Host != "192.168.1.1" {
		t.Errorf("expected Host '192.168.1.1', got %q", wire.Host)
	}
	if wire.Kind != domain.RecordA {
		t.Errorf("expected Kind RecordA, got %v", wire.Kind)
	}
	if wire.OwnerHostname != "docker-host" {
		t.Errorf("expected OwnerHostname 'docker-host', got %q", wire.OwnerHostname)
	}
	if wire.OwnerContainerId != "abc123" {
		t.Errorf("expected OwnerContainerId 'abc123', got %q", wire.OwnerContainerId)
	}
	if wire.OwnerContainerName != "my-app" {
		t.Errorf("expected OwnerContainerName 'my-app', got %q", wire.OwnerContainerName)
	}
	if !wire.Force {
		t.Error("expected Force to be true")
	}
}

func TestMarshalEtcdValue_ForceTrue(t *testing.T) {
	rec, _ := domain.NewA("app.example.com", "192.168.1.1")
	ri := &domain.RecordIntent{
		ContainerId:   "abc123",
		ContainerName: "my-app",
		Created:       time.Now(),
		Hostname:      "docker-host",
		Force:         true,
		Record:        rec,
	}

	result, err := marshalEtcdValue(ri)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var wire etcdRecord
	json.Unmarshal([]byte(result), &wire)

	if !wire.Force {
		t.Error("expected Force to be true in JSON")
	}
}

func TestMarshalEtcdValue_ForceFalse(t *testing.T) {
	rec, _ := domain.NewA("app.example.com", "192.168.1.1")
	ri := &domain.RecordIntent{
		ContainerId:   "abc123",
		ContainerName: "my-app",
		Created:       time.Now(),
		Hostname:      "docker-host",
		Force:         false,
		Record:        rec,
	}

	result, err := marshalEtcdValue(ri)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var wire etcdRecord
	json.Unmarshal([]byte(result), &wire)

	if wire.Force {
		t.Error("expected Force to be false in JSON")
	}
}

func TestMarshalEtcdValue_DifferentRecordTypes(t *testing.T) {
	tests := []struct {
		name     string
		kind     domain.RecordKind
		recName  string
		value    string
	}{
		{"A record", domain.RecordA, "app.example.com", "192.168.1.1"},
		{"AAAA record", domain.RecordAAAA, "app.example.com", "::1"},
		{"CNAME record", domain.RecordCNAME, "alias.example.com", "target.example.com"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var rec domain.Record
			switch tt.kind {
			case domain.RecordA:
				rec, _ = domain.NewA(tt.recName, tt.value)
			case domain.RecordAAAA:
				rec, _ = domain.NewAAAA(tt.recName, tt.value)
			case domain.RecordCNAME:
				rec, _ = domain.NewCNAME(tt.recName, tt.value)
			}

			ri := &domain.RecordIntent{
				ContainerId:   "abc123",
				ContainerName: "my-app",
				Created:       time.Now(),
				Hostname:      "docker-host",
				Force:         false,
				Record:        rec,
			}

			result, err := marshalEtcdValue(ri)

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			var wire etcdRecord
			json.Unmarshal([]byte(result), &wire)

			if wire.Kind != tt.kind {
				t.Errorf("expected Kind %v, got %v", tt.kind, wire.Kind)
			}
			if wire.Host != tt.value {
				t.Errorf("expected Host %q, got %q", tt.value, wire.Host)
			}
		})
	}
}

func TestUnmarshalEtcdValue_ValidJSON(t *testing.T) {
	key := "/skydns/com/example/app/x1"
	value := `{
		"host": "192.168.1.1",
		"record_type": "A",
		"owner_hostname": "docker-host",
		"owner_container_id": "abc123",
		"owner_container_name": "my-app",
		"created": "2024-01-15T10:30:00Z",
		"force": true
	}`

	result, err := unmarshalEtcdValue(key, value, "/skydns")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Record.Name != "app.example.com" {
		t.Errorf("expected Record.Name 'app.example.com', got %q", result.Record.Name)
	}
	if result.Record.Kind != domain.RecordA {
		t.Errorf("expected Record.Kind RecordA, got %v", result.Record.Kind)
	}
	if result.Record.Value != "192.168.1.1" {
		t.Errorf("expected Record.Value '192.168.1.1', got %q", result.Record.Value)
	}
	if result.ContainerId != "abc123" {
		t.Errorf("expected ContainerId 'abc123', got %q", result.ContainerId)
	}
	if result.ContainerName != "my-app" {
		t.Errorf("expected ContainerName 'my-app', got %q", result.ContainerName)
	}
	if result.Hostname != "docker-host" {
		t.Errorf("expected Hostname 'docker-host', got %q", result.Hostname)
	}
	if !result.Force {
		t.Error("expected Force to be true")
	}
}

func TestUnmarshalEtcdValue_InvalidJSON(t *testing.T) {
	key := "/skydns/com/example/app/x1"
	value := `not valid json`

	_, err := unmarshalEtcdValue(key, value, "/skydns")

	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestUnmarshalEtcdValue_InvalidRecord(t *testing.T) {
	key := "/skydns/com/example/app/x1"
	value := `{
		"host": "not-a-valid-ip",
		"record_type": "A",
		"owner_hostname": "docker-host",
		"owner_container_id": "abc123",
		"owner_container_name": "my-app",
		"created": "2024-01-15T10:30:00Z",
		"force": false
	}`

	_, err := unmarshalEtcdValue(key, value, "/skydns")

	if err == nil {
		t.Error("expected error for invalid record value")
	}
}

func TestMarshalUnmarshal_Roundtrip(t *testing.T) {
	created := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	rec, _ := domain.NewA("app.example.com", "192.168.1.1")
	original := &domain.RecordIntent{
		ContainerId:   "abc123",
		ContainerName: "my-app",
		Created:       created,
		Hostname:      "docker-host",
		Force:         true,
		Record:        rec,
	}

	// Marshal
	marshaled, err := marshalEtcdValue(original)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	// Unmarshal
	key := keyBaseForFQDN("/skydns", "app.example.com") + "/x1"
	result, err := unmarshalEtcdValue(key, marshaled, "/skydns")
	if err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	// Compare
	if result.ContainerId != original.ContainerId {
		t.Errorf("ContainerId mismatch: %q vs %q", result.ContainerId, original.ContainerId)
	}
	if result.ContainerName != original.ContainerName {
		t.Errorf("ContainerName mismatch: %q vs %q", result.ContainerName, original.ContainerName)
	}
	if result.Hostname != original.Hostname {
		t.Errorf("Hostname mismatch: %q vs %q", result.Hostname, original.Hostname)
	}
	if result.Force != original.Force {
		t.Errorf("Force mismatch: %v vs %v", result.Force, original.Force)
	}
	if !result.Record.Equal(original.Record) {
		t.Errorf("Record mismatch: %v vs %v", result.Record, original.Record)
	}
	if !result.Created.Equal(original.Created) {
		t.Errorf("Created mismatch: %v vs %v", result.Created, original.Created)
	}
}

func TestUnmarshalEtcdValue_AAAARecord(t *testing.T) {
	key := "/skydns/com/example/app/x1"
	value := `{
		"host": "2001:db8::1",
		"record_type": "AAAA",
		"owner_hostname": "docker-host",
		"owner_container_id": "abc123",
		"owner_container_name": "my-app",
		"created": "2024-01-15T10:30:00Z",
		"force": false
	}`

	result, err := unmarshalEtcdValue(key, value, "/skydns")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Record.Kind != domain.RecordAAAA {
		t.Errorf("expected RecordAAAA, got %v", result.Record.Kind)
	}
	if result.Record.Value != "2001:db8::1" {
		t.Errorf("expected value '2001:db8::1', got %q", result.Record.Value)
	}
}

func TestUnmarshalEtcdValue_CNAMERecord(t *testing.T) {
	key := "/skydns/com/example/alias/x1"
	value := `{
		"host": "target.example.com",
		"record_type": "CNAME",
		"owner_hostname": "docker-host",
		"owner_container_id": "abc123",
		"owner_container_name": "my-app",
		"created": "2024-01-15T10:30:00Z",
		"force": false
	}`

	result, err := unmarshalEtcdValue(key, value, "/skydns")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Record.Kind != domain.RecordCNAME {
		t.Errorf("expected RecordCNAME, got %v", result.Record.Kind)
	}
	if result.Record.Name != "alias.example.com" {
		t.Errorf("expected name 'alias.example.com', got %q", result.Record.Name)
	}
	if result.Record.Value != "target.example.com" {
		t.Errorf("expected value 'target.example.com', got %q", result.Record.Value)
	}
}

func TestEtcdRecord_JSONFieldNames(t *testing.T) {
	wire := etcdRecord{
		Host:               "192.168.1.1",
		Kind:               domain.RecordA,
		OwnerHostname:      "docker-host",
		OwnerContainerId:   "abc123",
		OwnerContainerName: "my-app",
		Created:            time.Now(),
		Force:              true,
	}

	data, err := json.Marshal(wire)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	jsonStr := string(data)

	// Verify JSON field names match expected format
	expectedFields := []string{
		`"host":`,
		`"record_type":`,
		`"owner_hostname":`,
		`"owner_container_id":`,
		`"owner_container_name":`,
		`"created":`,
		`"force":`,
	}

	for _, field := range expectedFields {
		if !contains(jsonStr, field) {
			t.Errorf("expected JSON to contain field %s, got: %s", field, jsonStr)
		}
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
