package domain

import (
	"strings"
	"testing"
	"time"
)

func TestRecordIntent_Key_Unique(t *testing.T) {
	now := time.Now()
	rec, _ := NewA("app.example.com", "192.168.1.1")

	ri1 := RecordIntent{
		ContainerId:   "container1",
		ContainerName: "app1",
		Created:       now,
		Hostname:      "host1",
		Force:         false,
		Record:        rec,
	}

	ri2 := RecordIntent{
		ContainerId:   "container2",
		ContainerName: "app1",
		Created:       now,
		Hostname:      "host1",
		Force:         false,
		Record:        rec,
	}

	ri3 := RecordIntent{
		ContainerId:   "container1",
		ContainerName: "app2",
		Created:       now,
		Hostname:      "host1",
		Force:         false,
		Record:        rec,
	}

	ri4 := RecordIntent{
		ContainerId:   "container1",
		ContainerName: "app1",
		Created:       now,
		Hostname:      "host2",
		Force:         false,
		Record:        rec,
	}

	ri5 := RecordIntent{
		ContainerId:   "container1",
		ContainerName: "app1",
		Created:       now,
		Hostname:      "host1",
		Force:         true,
		Record:        rec,
	}

	rec2, _ := NewA("app.example.com", "192.168.1.2")
	ri6 := RecordIntent{
		ContainerId:   "container1",
		ContainerName: "app1",
		Created:       now,
		Hostname:      "host1",
		Force:         false,
		Record:        rec2,
	}

	keys := map[string]bool{
		ri1.Key(): true,
		ri2.Key(): true,
		ri3.Key(): true,
		ri4.Key(): true,
		ri5.Key(): true,
		ri6.Key(): true,
	}

	if len(keys) != 6 {
		t.Errorf("expected 6 unique keys, got %d", len(keys))
	}
}

func TestRecordIntent_Key_SameForIdentical(t *testing.T) {
	now := time.Now()
	rec, _ := NewA("app.example.com", "192.168.1.1")

	ri1 := RecordIntent{
		ContainerId:   "container1",
		ContainerName: "app1",
		Created:       now,
		Hostname:      "host1",
		Force:         false,
		Record:        rec,
	}

	ri2 := RecordIntent{
		ContainerId:   "container1",
		ContainerName: "app1",
		Created:       now.Add(time.Hour), // Different Created, but not part of key
		Hostname:      "host1",
		Force:         false,
		Record:        rec,
	}

	if ri1.Key() != ri2.Key() {
		t.Errorf("expected identical keys (Created is not part of key): %q vs %q", ri1.Key(), ri2.Key())
	}
}

func TestRecordIntent_Equal_AllFieldsMatch(t *testing.T) {
	now := time.Now()
	rec, _ := NewA("app.example.com", "192.168.1.1")

	ri1 := RecordIntent{
		ContainerId:   "container1",
		ContainerName: "app1",
		Created:       now,
		Hostname:      "host1",
		Force:         false,
		Record:        rec,
	}

	ri2 := RecordIntent{
		ContainerId:   "container1",
		ContainerName: "app1",
		Created:       now,
		Hostname:      "host1",
		Force:         false,
		Record:        rec,
	}

	if !ri1.Equal(ri2) {
		t.Error("expected identical RecordIntents to be equal")
	}
}

func TestRecordIntent_Equal_DifferentContainerId(t *testing.T) {
	now := time.Now()
	rec, _ := NewA("app.example.com", "192.168.1.1")

	ri1 := RecordIntent{
		ContainerId:   "container1",
		ContainerName: "app1",
		Created:       now,
		Hostname:      "host1",
		Force:         false,
		Record:        rec,
	}

	ri2 := RecordIntent{
		ContainerId:   "container2",
		ContainerName: "app1",
		Created:       now,
		Hostname:      "host1",
		Force:         false,
		Record:        rec,
	}

	if ri1.Equal(ri2) {
		t.Error("expected RecordIntents with different ContainerId to not be equal")
	}
}

func TestRecordIntent_Equal_DifferentForce(t *testing.T) {
	now := time.Now()
	rec, _ := NewA("app.example.com", "192.168.1.1")

	ri1 := RecordIntent{
		ContainerId:   "container1",
		ContainerName: "app1",
		Created:       now,
		Hostname:      "host1",
		Force:         false,
		Record:        rec,
	}

	ri2 := RecordIntent{
		ContainerId:   "container1",
		ContainerName: "app1",
		Created:       now,
		Hostname:      "host1",
		Force:         true,
		Record:        rec,
	}

	if ri1.Equal(ri2) {
		t.Error("expected RecordIntents with different Force to not be equal")
	}
}

func TestRecordIntent_Equal_DifferentRecord(t *testing.T) {
	now := time.Now()
	rec1, _ := NewA("app.example.com", "192.168.1.1")
	rec2, _ := NewA("app.example.com", "192.168.1.2")

	ri1 := RecordIntent{
		ContainerId:   "container1",
		ContainerName: "app1",
		Created:       now,
		Hostname:      "host1",
		Force:         false,
		Record:        rec1,
	}

	ri2 := RecordIntent{
		ContainerId:   "container1",
		ContainerName: "app1",
		Created:       now,
		Hostname:      "host1",
		Force:         false,
		Record:        rec2,
	}

	if ri1.Equal(ri2) {
		t.Error("expected RecordIntents with different Record to not be equal")
	}
}

func TestRecordIntent_Equal_IgnoresCreated(t *testing.T) {
	rec, _ := NewA("app.example.com", "192.168.1.1")

	ri1 := RecordIntent{
		ContainerId:   "container1",
		ContainerName: "app1",
		Created:       time.Now(),
		Hostname:      "host1",
		Force:         false,
		Record:        rec,
	}

	ri2 := RecordIntent{
		ContainerId:   "container1",
		ContainerName: "app1",
		Created:       time.Now().Add(time.Hour),
		Hostname:      "host1",
		Force:         false,
		Record:        rec,
	}

	// Note: Looking at the Equal implementation, it does NOT compare Created
	// This test verifies that behavior
	if !ri1.Equal(ri2) {
		t.Error("expected RecordIntents with different Created to still be equal (Created not compared)")
	}
}

func TestRecordIntent_Render_IncludesAllInfo(t *testing.T) {
	created := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	rec, _ := NewA("app.example.com", "192.168.1.1")

	ri := RecordIntent{
		ContainerId:   "abc123",
		ContainerName: "myapp",
		Created:       created,
		Hostname:      "docker-host",
		Force:         true,
		Record:        rec,
	}

	rendered := ri.Render()

	// Check that all important info is included
	checks := []string{
		"[A]",
		"app.example.com",
		"192.168.1.1",
		"container_id=abc123",
		"container_name=myapp",
		"hostname=docker-host",
		"force=true",
		"2024-01-15",
	}

	for _, check := range checks {
		if !strings.Contains(rendered, check) {
			t.Errorf("expected Render() to contain %q, got: %s", check, rendered)
		}
	}
}

func TestRecordIntent_Render_ForceFalse(t *testing.T) {
	rec, _ := NewA("app.example.com", "192.168.1.1")

	ri := RecordIntent{
		ContainerId:   "abc123",
		ContainerName: "myapp",
		Created:       time.Now(),
		Hostname:      "docker-host",
		Force:         false,
		Record:        rec,
	}

	rendered := ri.Render()

	if !strings.Contains(rendered, "force=false") {
		t.Errorf("expected Render() to contain 'force=false', got: %s", rendered)
	}
}
