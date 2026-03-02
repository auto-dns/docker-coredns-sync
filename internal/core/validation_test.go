package core

import (
	"testing"
	"time"

	"github.com/auto-dns/docker-coredns-sync/internal/domain"
	"github.com/rs/zerolog"
)

func testLogger() zerolog.Logger {
	return zerolog.Nop()
}

func makeIntent(name string, kind domain.RecordKind, value string) *domain.RecordIntent {
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

func TestValidateRecord_ANoConflict(t *testing.T) {
	newRI := makeIntent("app.example.com", domain.RecordA, "192.168.1.1")
	existing := []*domain.RecordIntent{}

	err := ValidateRecord(newRI, existing, testLogger())

	if err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
}

func TestValidateRecord_AWithExistingA_DifferentValue(t *testing.T) {
	newRI := makeIntent("app.example.com", domain.RecordA, "192.168.1.2")
	existing := []*domain.RecordIntent{
		makeIntent("app.example.com", domain.RecordA, "192.168.1.1"),
	}

	err := ValidateRecord(newRI, existing, testLogger())

	if err != nil {
		t.Errorf("expected no error for different A values, got: %v", err)
	}
}

func TestValidateRecord_AWithExistingCNAME(t *testing.T) {
	newRI := makeIntent("app.example.com", domain.RecordA, "192.168.1.1")
	existing := []*domain.RecordIntent{
		makeIntent("app.example.com", domain.RecordCNAME, "target.example.com"),
	}

	err := ValidateRecord(newRI, existing, testLogger())

	if err == nil {
		t.Error("expected error when A conflicts with existing CNAME")
	}
}

func TestValidateRecord_AAAAWithExistingCNAME(t *testing.T) {
	newRI := makeIntent("app.example.com", domain.RecordAAAA, "::1")
	existing := []*domain.RecordIntent{
		makeIntent("app.example.com", domain.RecordCNAME, "target.example.com"),
	}

	err := ValidateRecord(newRI, existing, testLogger())

	if err == nil {
		t.Error("expected error when AAAA conflicts with existing CNAME")
	}
}

func TestValidateRecord_CNAMEWithExistingA(t *testing.T) {
	newRI := makeIntent("app.example.com", domain.RecordCNAME, "target.example.com")
	existing := []*domain.RecordIntent{
		makeIntent("app.example.com", domain.RecordA, "192.168.1.1"),
	}

	err := ValidateRecord(newRI, existing, testLogger())

	if err == nil {
		t.Error("expected error when CNAME conflicts with existing A")
	}
}

func TestValidateRecord_CNAMEWithExistingAAAA(t *testing.T) {
	newRI := makeIntent("app.example.com", domain.RecordCNAME, "target.example.com")
	existing := []*domain.RecordIntent{
		makeIntent("app.example.com", domain.RecordAAAA, "::1"),
	}

	err := ValidateRecord(newRI, existing, testLogger())

	if err == nil {
		t.Error("expected error when CNAME conflicts with existing AAAA")
	}
}

func TestValidateRecord_CNAMEDuplicate(t *testing.T) {
	newRI := makeIntent("app.example.com", domain.RecordCNAME, "target2.example.com")
	existing := []*domain.RecordIntent{
		makeIntent("app.example.com", domain.RecordCNAME, "target1.example.com"),
	}

	err := ValidateRecord(newRI, existing, testLogger())

	if err == nil {
		t.Error("expected error for duplicate CNAME")
	}
}

func TestValidateRecord_DuplicateAValue(t *testing.T) {
	newRI := makeIntent("app.example.com", domain.RecordA, "192.168.1.1")
	existing := []*domain.RecordIntent{
		makeIntent("app.example.com", domain.RecordA, "192.168.1.1"),
	}

	err := ValidateRecord(newRI, existing, testLogger())

	if err == nil {
		t.Error("expected error for duplicate A value")
	}
}

func TestValidateRecord_DuplicateAAAAValue(t *testing.T) {
	newRI := makeIntent("app.example.com", domain.RecordAAAA, "::1")
	existing := []*domain.RecordIntent{
		makeIntent("app.example.com", domain.RecordAAAA, "::1"),
	}

	err := ValidateRecord(newRI, existing, testLogger())

	if err == nil {
		t.Error("expected error for duplicate AAAA value")
	}
}

func TestValidateRecord_MultipleADifferentValues(t *testing.T) {
	newRI := makeIntent("app.example.com", domain.RecordA, "192.168.1.3")
	existing := []*domain.RecordIntent{
		makeIntent("app.example.com", domain.RecordA, "192.168.1.1"),
		makeIntent("app.example.com", domain.RecordA, "192.168.1.2"),
	}

	err := ValidateRecord(newRI, existing, testLogger())

	if err != nil {
		t.Errorf("expected no error for multiple A with different values, got: %v", err)
	}
}

func TestValidateRecord_CNAMECycleSimple(t *testing.T) {
	// Create cycle: A -> B -> A
	newRI := makeIntent("b.example.com", domain.RecordCNAME, "a.example.com")
	existing := []*domain.RecordIntent{
		makeIntent("a.example.com", domain.RecordCNAME, "b.example.com"),
	}

	err := ValidateRecord(newRI, existing, testLogger())

	if err == nil {
		t.Error("expected error for CNAME cycle")
	}
}

func TestValidateRecord_CNAMECycleLong(t *testing.T) {
	// Create cycle: A -> B -> C -> A
	newRI := makeIntent("c.example.com", domain.RecordCNAME, "a.example.com")
	existing := []*domain.RecordIntent{
		makeIntent("a.example.com", domain.RecordCNAME, "b.example.com"),
		makeIntent("b.example.com", domain.RecordCNAME, "c.example.com"),
	}

	err := ValidateRecord(newRI, existing, testLogger())

	if err == nil {
		t.Error("expected error for long CNAME cycle")
	}
}

func TestValidateRecord_CNAMENoCycle(t *testing.T) {
	// Chain without cycle: A -> B -> C (no back reference)
	newRI := makeIntent("c.example.com", domain.RecordCNAME, "d.example.com")
	existing := []*domain.RecordIntent{
		makeIntent("a.example.com", domain.RecordCNAME, "b.example.com"),
		makeIntent("b.example.com", domain.RecordCNAME, "c.example.com"),
	}

	err := ValidateRecord(newRI, existing, testLogger())

	if err != nil {
		t.Errorf("expected no error for non-cyclic CNAME chain, got: %v", err)
	}
}

func TestValidateRecord_CNAMESelfReference(t *testing.T) {
	// Self-referencing CNAME: A -> A
	newRI := makeIntent("a.example.com", domain.RecordCNAME, "a.example.com")
	existing := []*domain.RecordIntent{}

	err := ValidateRecord(newRI, existing, testLogger())

	if err == nil {
		t.Error("expected error for self-referencing CNAME")
	}
}

func TestValidateRecord_DifferentNames_NoConflict(t *testing.T) {
	newRI := makeIntent("app1.example.com", domain.RecordA, "192.168.1.1")
	existing := []*domain.RecordIntent{
		makeIntent("app2.example.com", domain.RecordCNAME, "target.example.com"),
		makeIntent("app3.example.com", domain.RecordA, "192.168.1.1"),
	}

	err := ValidateRecord(newRI, existing, testLogger())

	if err != nil {
		t.Errorf("expected no error for different names, got: %v", err)
	}
}

func TestValidateRecord_AAndAAAACanCoexist(t *testing.T) {
	newRI := makeIntent("app.example.com", domain.RecordAAAA, "::1")
	existing := []*domain.RecordIntent{
		makeIntent("app.example.com", domain.RecordA, "192.168.1.1"),
	}

	err := ValidateRecord(newRI, existing, testLogger())

	if err != nil {
		t.Errorf("expected no error for A and AAAA coexisting, got: %v", err)
	}
}

func TestValidateRecord_MultipleAAAADifferentValues(t *testing.T) {
	newRI := makeIntent("app.example.com", domain.RecordAAAA, "::3")
	existing := []*domain.RecordIntent{
		makeIntent("app.example.com", domain.RecordAAAA, "::1"),
		makeIntent("app.example.com", domain.RecordAAAA, "::2"),
	}

	err := ValidateRecord(newRI, existing, testLogger())

	if err != nil {
		t.Errorf("expected no error for multiple AAAA with different values, got: %v", err)
	}
}

func TestValidateRecord_CNAMENoExistingRecords(t *testing.T) {
	newRI := makeIntent("alias.example.com", domain.RecordCNAME, "target.example.com")
	existing := []*domain.RecordIntent{}

	err := ValidateRecord(newRI, existing, testLogger())

	if err != nil {
		t.Errorf("expected no error for new CNAME, got: %v", err)
	}
}

func TestValidateRecord_CNAMEWithUnrelatedExisting(t *testing.T) {
	newRI := makeIntent("alias.example.com", domain.RecordCNAME, "target.example.com")
	existing := []*domain.RecordIntent{
		makeIntent("other.example.com", domain.RecordA, "192.168.1.1"),
		makeIntent("another.example.com", domain.RecordCNAME, "somewhere.example.com"),
	}

	err := ValidateRecord(newRI, existing, testLogger())

	if err != nil {
		t.Errorf("expected no error for CNAME with unrelated existing, got: %v", err)
	}
}
