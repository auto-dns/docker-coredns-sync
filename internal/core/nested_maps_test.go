package core

import (
	"testing"
	"time"

	"github.com/auto-dns/docker-coredns-sync/internal/domain"
)

func makeTestIntent(name string, kind domain.RecordKind, value string) *domain.RecordIntent {
	var rec domain.Record
	switch kind {
	case domain.RecordA:
		rec, _ = domain.NewA(name, value)
	case domain.RecordAAAA:
		rec, _ = domain.NewAAAA(name, value)
	case domain.RecordCNAME:
		rec, _ = domain.NewCNAME(name, value)
	}
	return &domain.RecordIntent{
		ContainerId:   "test-container",
		ContainerName: "test",
		Created:       time.Now(),
		Hostname:      "test-host",
		Force:         false,
		Record:        rec,
	}
}

func TestNestedRecordMap_GetCreatesHierarchy(t *testing.T) {
	m := newNestedRecordMap()

	// Get should create nested structure
	kindMap := m.Get("example.com")
	if kindMap == nil {
		t.Fatal("expected kindMap to be created")
	}

	recordMap := kindMap.Get(domain.RecordA)
	if recordMap == nil {
		t.Fatal("expected recordMap to be created")
	}
}

func TestNestedRecordMap_SetAndRetrieve(t *testing.T) {
	m := newNestedRecordMap()
	intent := makeTestIntent("app.example.com", domain.RecordA, "192.168.1.1")

	m.Get("app.example.com").Get(domain.RecordA).Set("192.168.1.1", intent)

	retrieved := m.Get("app.example.com").Get(domain.RecordA).Get("192.168.1.1")
	if retrieved != intent {
		t.Error("expected to retrieve the same intent")
	}
}

func TestNestedRecordMap_PeekNameKindRecords_Exists(t *testing.T) {
	m := newNestedRecordMap()
	intent1 := makeTestIntent("app.example.com", domain.RecordA, "192.168.1.1")
	intent2 := makeTestIntent("app.example.com", domain.RecordA, "192.168.1.2")

	m.Get("app.example.com").Get(domain.RecordA).Set("192.168.1.1", intent1)
	m.Get("app.example.com").Get(domain.RecordA).Set("192.168.1.2", intent2)

	records, exists := m.PeekNameKindRecords("app.example.com", domain.RecordA)

	if !exists {
		t.Error("expected records to exist")
	}
	if len(records) != 2 {
		t.Errorf("expected 2 records, got %d", len(records))
	}
}

func TestNestedRecordMap_PeekNameKindRecords_NotExists_NoName(t *testing.T) {
	m := newNestedRecordMap()

	records, exists := m.PeekNameKindRecords("nonexistent.com", domain.RecordA)

	if exists {
		t.Error("expected exists to be false")
	}
	if len(records) != 0 {
		t.Errorf("expected 0 records, got %d", len(records))
	}
}

func TestNestedRecordMap_PeekNameKindRecords_NotExists_NoKind(t *testing.T) {
	m := newNestedRecordMap()
	intent := makeTestIntent("app.example.com", domain.RecordA, "192.168.1.1")
	m.Get("app.example.com").Get(domain.RecordA).Set("192.168.1.1", intent)

	records, exists := m.PeekNameKindRecords("app.example.com", domain.RecordAAAA)

	if exists {
		t.Error("expected exists to be false for non-existent kind")
	}
	if len(records) != 0 {
		t.Errorf("expected 0 records, got %d", len(records))
	}
}

func TestNestedRecordMap_PeekNameKindRecord_Exists(t *testing.T) {
	m := newNestedRecordMap()
	intent := makeTestIntent("app.example.com", domain.RecordA, "192.168.1.1")
	m.Get("app.example.com").Get(domain.RecordA).Set("192.168.1.1", intent)

	record, exists := m.PeekNameKindRecord("app.example.com", domain.RecordA, "192.168.1.1")

	if !exists {
		t.Error("expected record to exist")
	}
	if record != intent {
		t.Error("expected to get the same intent")
	}
}

func TestNestedRecordMap_PeekNameKindRecord_NotExists(t *testing.T) {
	m := newNestedRecordMap()

	record, exists := m.PeekNameKindRecord("app.example.com", domain.RecordA, "192.168.1.1")

	if exists {
		t.Error("expected exists to be false")
	}
	if record != nil {
		t.Error("expected record to be nil")
	}
}

func TestNestedRecordMap_GetAllValues(t *testing.T) {
	m := newNestedRecordMap()
	intent1 := makeTestIntent("app1.example.com", domain.RecordA, "192.168.1.1")
	intent2 := makeTestIntent("app2.example.com", domain.RecordA, "192.168.1.2")
	intent3 := makeTestIntent("app1.example.com", domain.RecordAAAA, "::1")
	intent4 := makeTestIntent("alias.example.com", domain.RecordCNAME, "app1.example.com")

	m.Get("app1.example.com").Get(domain.RecordA).Set("192.168.1.1", intent1)
	m.Get("app2.example.com").Get(domain.RecordA).Set("192.168.1.2", intent2)
	m.Get("app1.example.com").Get(domain.RecordAAAA).Set("::1", intent3)
	m.Get("alias.example.com").Get(domain.RecordCNAME).Set("app1.example.com", intent4)

	values := m.GetAllValues()

	if len(values) != 4 {
		t.Errorf("expected 4 values, got %d", len(values))
	}
}

func TestNestedRecordMap_GetAllValues_Empty(t *testing.T) {
	m := newNestedRecordMap()

	values := m.GetAllValues()

	if len(values) != 0 {
		t.Errorf("expected 0 values, got %d", len(values))
	}
}

func TestNestedRecordMap_GetAllNames(t *testing.T) {
	m := newNestedRecordMap()
	intent1 := makeTestIntent("app1.example.com", domain.RecordA, "192.168.1.1")
	intent2 := makeTestIntent("app2.example.com", domain.RecordA, "192.168.1.2")
	intent3 := makeTestIntent("app1.example.com", domain.RecordAAAA, "::1")

	m.Get("app1.example.com").Get(domain.RecordA).Set("192.168.1.1", intent1)
	m.Get("app2.example.com").Get(domain.RecordA).Set("192.168.1.2", intent2)
	m.Get("app1.example.com").Get(domain.RecordAAAA).Set("::1", intent3)

	names := m.GetAllNames()

	if len(names) != 2 {
		t.Errorf("expected 2 unique names, got %d", len(names))
	}

	nameSet := make(map[string]bool)
	for _, n := range names {
		nameSet[n] = true
	}

	if !nameSet["app1.example.com"] {
		t.Error("expected 'app1.example.com' in names")
	}
	if !nameSet["app2.example.com"] {
		t.Error("expected 'app2.example.com' in names")
	}
}

func TestNestedRecordMap_GetAllNames_Empty(t *testing.T) {
	m := newNestedRecordMap()

	names := m.GetAllNames()

	if len(names) != 0 {
		t.Errorf("expected 0 names, got %d", len(names))
	}
}

func TestNestedRecordMap_MultipleKindsPerName(t *testing.T) {
	m := newNestedRecordMap()
	intentA := makeTestIntent("app.example.com", domain.RecordA, "192.168.1.1")
	intentAAAA := makeTestIntent("app.example.com", domain.RecordAAAA, "::1")

	m.Get("app.example.com").Get(domain.RecordA).Set("192.168.1.1", intentA)
	m.Get("app.example.com").Get(domain.RecordAAAA).Set("::1", intentAAAA)

	aRecords, aExists := m.PeekNameKindRecords("app.example.com", domain.RecordA)
	aaaaRecords, aaaaExists := m.PeekNameKindRecords("app.example.com", domain.RecordAAAA)

	if !aExists || len(aRecords) != 1 {
		t.Error("expected 1 A record")
	}
	if !aaaaExists || len(aaaaRecords) != 1 {
		t.Error("expected 1 AAAA record")
	}
}

func TestNestedRecordMap_MultipleValuesPerKind(t *testing.T) {
	m := newNestedRecordMap()
	intent1 := makeTestIntent("app.example.com", domain.RecordA, "192.168.1.1")
	intent2 := makeTestIntent("app.example.com", domain.RecordA, "192.168.1.2")
	intent3 := makeTestIntent("app.example.com", domain.RecordA, "192.168.1.3")

	m.Get("app.example.com").Get(domain.RecordA).Set("192.168.1.1", intent1)
	m.Get("app.example.com").Get(domain.RecordA).Set("192.168.1.2", intent2)
	m.Get("app.example.com").Get(domain.RecordA).Set("192.168.1.3", intent3)

	records, exists := m.PeekNameKindRecords("app.example.com", domain.RecordA)

	if !exists {
		t.Error("expected records to exist")
	}
	if len(records) != 3 {
		t.Errorf("expected 3 A records, got %d", len(records))
	}
}
