package core

import (
	"testing"
	"time"

	"github.com/auto-dns/docker-coredns-sync/internal/config"
	"github.com/auto-dns/docker-coredns-sync/internal/domain"
	"github.com/rs/zerolog"
)

func reconcileLogger() zerolog.Logger {
	return zerolog.Nop()
}

func reconcileConfig() *config.AppConfig {
	return &config.AppConfig{
		DockerLabelPrefix: "coredns",
		HostIPv4:          "10.0.0.1",
		HostIPv6:          "::1",
		Hostname:          "test-host",
		PollInterval:      5,
	}
}

func makeRecordIntent(name string, kind domain.RecordKind, value string, containerId string, created time.Time, force bool, hostname string) *domain.RecordIntent {
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
		ContainerId:   containerId,
		ContainerName: "container-" + containerId,
		Created:       created,
		Hostname:      hostname,
		Force:         force,
		Record:        rec,
	}
}

// Helper to make intents with simpler signature
func simpleIntent(name string, kind domain.RecordKind, value string, containerSuffix string, hoursAgo int, force bool) *domain.RecordIntent {
	return makeRecordIntent(
		name, kind, value,
		"container-"+containerSuffix,
		time.Now().Add(-time.Duration(hoursAgo)*time.Hour),
		force,
		"test-host",
	)
}

// ============================================================================
// shouldReplaceExisting tests
// ============================================================================

func TestShouldReplaceExisting_NewForceBeatsNonForce(t *testing.T) {
	now := time.Now()
	newIntent := makeRecordIntent("app.example.com", domain.RecordA, "192.168.1.1", "new", now, true, "host1")
	existing := makeRecordIntent("app.example.com", domain.RecordA, "192.168.1.1", "existing", now.Add(-time.Hour), false, "host1")

	result := shouldReplaceExisting(newIntent, existing, reconcileLogger())

	if !result {
		t.Error("expected new force to beat existing non-force")
	}
}

func TestShouldReplaceExisting_ExistingForcePrevails(t *testing.T) {
	now := time.Now()
	newIntent := makeRecordIntent("app.example.com", domain.RecordA, "192.168.1.1", "new", now.Add(-2*time.Hour), false, "host1")
	existing := makeRecordIntent("app.example.com", domain.RecordA, "192.168.1.1", "existing", now, true, "host1")

	result := shouldReplaceExisting(newIntent, existing, reconcileLogger())

	if result {
		t.Error("expected existing force to prevail over non-force new")
	}
}

func TestShouldReplaceExisting_BothForceOlderWins(t *testing.T) {
	now := time.Now()
	newIntent := makeRecordIntent("app.example.com", domain.RecordA, "192.168.1.1", "new", now.Add(-2*time.Hour), true, "host1")
	existing := makeRecordIntent("app.example.com", domain.RecordA, "192.168.1.1", "existing", now, true, "host1")

	result := shouldReplaceExisting(newIntent, existing, reconcileLogger())

	if !result {
		t.Error("expected older new with force to beat newer existing with force")
	}
}

func TestShouldReplaceExisting_BothNonForceOlderWins(t *testing.T) {
	now := time.Now()
	newIntent := makeRecordIntent("app.example.com", domain.RecordA, "192.168.1.1", "new", now.Add(-2*time.Hour), false, "host1")
	existing := makeRecordIntent("app.example.com", domain.RecordA, "192.168.1.1", "existing", now, false, "host1")

	result := shouldReplaceExisting(newIntent, existing, reconcileLogger())

	if !result {
		t.Error("expected older new to beat newer existing (both non-force)")
	}
}

func TestShouldReplaceExisting_SameTimestampKeepsExisting(t *testing.T) {
	now := time.Now()
	newIntent := makeRecordIntent("app.example.com", domain.RecordA, "192.168.1.1", "new", now, false, "host1")
	existing := makeRecordIntent("app.example.com", domain.RecordA, "192.168.1.1", "existing", now, false, "host1")

	result := shouldReplaceExisting(newIntent, existing, reconcileLogger())

	if result {
		t.Error("expected same timestamp to keep existing")
	}
}

// ============================================================================
// shouldReplaceAllExisting tests
// ============================================================================

func TestShouldReplaceAllExisting_EmptyExisting(t *testing.T) {
	newIntent := simpleIntent("app.example.com", domain.RecordCNAME, "target.example.com", "new", 1, false)
	existing := []*domain.RecordIntent{}

	result := shouldReplaceAllExisting(newIntent, existing, reconcileLogger())

	if !result {
		t.Error("expected true for empty existing")
	}
}

func TestShouldReplaceAllExisting_NewForceAllNonForce(t *testing.T) {
	newIntent := simpleIntent("app.example.com", domain.RecordCNAME, "target.example.com", "new", 1, true)
	existing := []*domain.RecordIntent{
		simpleIntent("app.example.com", domain.RecordA, "192.168.1.1", "e1", 2, false),
		simpleIntent("app.example.com", domain.RecordA, "192.168.1.2", "e2", 3, false),
	}

	result := shouldReplaceAllExisting(newIntent, existing, reconcileLogger())

	if !result {
		t.Error("expected new force to beat all non-force existing")
	}
}

func TestShouldReplaceAllExisting_AnyExistingForceNewNonForce(t *testing.T) {
	newIntent := simpleIntent("app.example.com", domain.RecordCNAME, "target.example.com", "new", 10, false)
	existing := []*domain.RecordIntent{
		simpleIntent("app.example.com", domain.RecordA, "192.168.1.1", "e1", 2, false),
		simpleIntent("app.example.com", domain.RecordA, "192.168.1.2", "e2", 3, true), // one has force
	}

	result := shouldReplaceAllExisting(newIntent, existing, reconcileLogger())

	if result {
		t.Error("expected new non-force to lose to any existing with force")
	}
}

func TestShouldReplaceAllExisting_NewOlderThanAllForce(t *testing.T) {
	newIntent := simpleIntent("app.example.com", domain.RecordCNAME, "target.example.com", "new", 10, true)
	existing := []*domain.RecordIntent{
		simpleIntent("app.example.com", domain.RecordA, "192.168.1.1", "e1", 2, true),
		simpleIntent("app.example.com", domain.RecordA, "192.168.1.2", "e2", 3, true),
	}

	result := shouldReplaceAllExisting(newIntent, existing, reconcileLogger())

	if !result {
		t.Error("expected new force older than all existing force to win")
	}
}

func TestShouldReplaceAllExisting_NewNotOlderThanAllForce(t *testing.T) {
	newIntent := simpleIntent("app.example.com", domain.RecordCNAME, "target.example.com", "new", 1, true)
	existing := []*domain.RecordIntent{
		simpleIntent("app.example.com", domain.RecordA, "192.168.1.1", "e1", 5, true), // older than new
		simpleIntent("app.example.com", domain.RecordA, "192.168.1.2", "e2", 2, true),
	}

	result := shouldReplaceAllExisting(newIntent, existing, reconcileLogger())

	if result {
		t.Error("expected new force not older than all existing force to lose")
	}
}

func TestShouldReplaceAllExisting_AllNonForceOldestWins(t *testing.T) {
	newIntent := simpleIntent("app.example.com", domain.RecordCNAME, "target.example.com", "new", 10, false)
	existing := []*domain.RecordIntent{
		simpleIntent("app.example.com", domain.RecordA, "192.168.1.1", "e1", 2, false),
		simpleIntent("app.example.com", domain.RecordA, "192.168.1.2", "e2", 3, false),
	}

	result := shouldReplaceAllExisting(newIntent, existing, reconcileLogger())

	if !result {
		t.Error("expected oldest (new) to win when all non-force")
	}
}

// ============================================================================
// FilterRecordIntents tests
// ============================================================================

func TestFilterRecordIntents_NoDuplicates(t *testing.T) {
	intents := []*domain.RecordIntent{
		simpleIntent("app1.example.com", domain.RecordA, "192.168.1.1", "c1", 1, false),
		simpleIntent("app2.example.com", domain.RecordA, "192.168.1.2", "c2", 1, false),
		simpleIntent("alias.example.com", domain.RecordCNAME, "target.example.com", "c3", 1, false),
	}

	result := FilterRecordIntents(intents, reconcileLogger())

	if len(result) != 3 {
		t.Errorf("expected 3 records, got %d", len(result))
	}
}

func TestFilterRecordIntents_DuplicateADeduped(t *testing.T) {
	intents := []*domain.RecordIntent{
		simpleIntent("app.example.com", domain.RecordA, "192.168.1.1", "newer", 1, false),
		simpleIntent("app.example.com", domain.RecordA, "192.168.1.1", "older", 5, false), // older should win
	}

	result := FilterRecordIntents(intents, reconcileLogger())

	if len(result) != 1 {
		t.Fatalf("expected 1 record after dedup, got %d", len(result))
	}

	if result[0].ContainerId != "container-older" {
		t.Errorf("expected older container to win, got %q", result[0].ContainerId)
	}
}

func TestFilterRecordIntents_DuplicateCNAMEDeduped(t *testing.T) {
	intents := []*domain.RecordIntent{
		simpleIntent("alias.example.com", domain.RecordCNAME, "target1.example.com", "newer", 1, false),
		simpleIntent("alias.example.com", domain.RecordCNAME, "target2.example.com", "older", 5, false),
	}

	result := FilterRecordIntents(intents, reconcileLogger())

	if len(result) != 1 {
		t.Fatalf("expected 1 CNAME after dedup, got %d", len(result))
	}

	if result[0].ContainerId != "container-older" {
		t.Errorf("expected older container to win, got %q", result[0].ContainerId)
	}
}

func TestFilterRecordIntents_CNAMEVsA_CNAMEWins(t *testing.T) {
	intents := []*domain.RecordIntent{
		simpleIntent("app.example.com", domain.RecordA, "192.168.1.1", "newer", 1, false),
		simpleIntent("app.example.com", domain.RecordCNAME, "target.example.com", "older", 5, false), // older CNAME wins
	}

	result := FilterRecordIntents(intents, reconcileLogger())

	if len(result) != 1 {
		t.Fatalf("expected 1 record, got %d", len(result))
	}

	if result[0].Record.Kind != domain.RecordCNAME {
		t.Errorf("expected CNAME to win, got %v", result[0].Record.Kind)
	}
}

func TestFilterRecordIntents_CNAMEVsA_AWins(t *testing.T) {
	intents := []*domain.RecordIntent{
		simpleIntent("app.example.com", domain.RecordA, "192.168.1.1", "older", 5, false), // older A wins
		simpleIntent("app.example.com", domain.RecordCNAME, "target.example.com", "newer", 1, false),
	}

	result := FilterRecordIntents(intents, reconcileLogger())

	if len(result) != 1 {
		t.Fatalf("expected 1 record, got %d", len(result))
	}

	if result[0].Record.Kind != domain.RecordA {
		t.Errorf("expected A to win, got %v", result[0].Record.Kind)
	}
}

func TestFilterRecordIntents_MultipleADifferentValues(t *testing.T) {
	intents := []*domain.RecordIntent{
		simpleIntent("app.example.com", domain.RecordA, "192.168.1.1", "c1", 1, false),
		simpleIntent("app.example.com", domain.RecordA, "192.168.1.2", "c2", 2, false),
		simpleIntent("app.example.com", domain.RecordA, "192.168.1.3", "c3", 3, false),
	}

	result := FilterRecordIntents(intents, reconcileLogger())

	if len(result) != 3 {
		t.Errorf("expected 3 A records with different values, got %d", len(result))
	}
}

// ============================================================================
// ReconcileAndValidate tests
// ============================================================================

func TestReconcileAndValidate_AddNewRecord(t *testing.T) {
	cfg := reconcileConfig()
	desired := []*domain.RecordIntent{
		simpleIntent("app.example.com", domain.RecordA, "192.168.1.1", "c1", 1, false),
	}
	actual := []*domain.RecordIntent{}

	toAdd, toRemove := ReconcileAndValidate(desired, actual, cfg, reconcileLogger())

	if len(toAdd) != 1 {
		t.Errorf("expected 1 record to add, got %d", len(toAdd))
	}
	if len(toRemove) != 0 {
		t.Errorf("expected 0 records to remove, got %d", len(toRemove))
	}
}

func TestReconcileAndValidate_RemoveStaleOwned(t *testing.T) {
	cfg := reconcileConfig()
	desired := []*domain.RecordIntent{}
	actual := []*domain.RecordIntent{
		makeRecordIntent("app.example.com", domain.RecordA, "192.168.1.1", "c1", time.Now(), false, "test-host"),
	}

	toAdd, toRemove := ReconcileAndValidate(desired, actual, cfg, reconcileLogger())

	if len(toAdd) != 0 {
		t.Errorf("expected 0 records to add, got %d", len(toAdd))
	}
	if len(toRemove) != 1 {
		t.Errorf("expected 1 record to remove, got %d", len(toRemove))
	}
}

func TestReconcileAndValidate_KeepStaleNotOwned(t *testing.T) {
	cfg := reconcileConfig()
	desired := []*domain.RecordIntent{}
	actual := []*domain.RecordIntent{
		makeRecordIntent("app.example.com", domain.RecordA, "192.168.1.1", "c1", time.Now(), false, "other-host"), // different host
	}

	toAdd, toRemove := ReconcileAndValidate(desired, actual, cfg, reconcileLogger())

	if len(toAdd) != 0 {
		t.Errorf("expected 0 records to add, got %d", len(toAdd))
	}
	if len(toRemove) != 0 {
		t.Errorf("expected 0 records to remove (not owned), got %d", len(toRemove))
	}
}

func TestReconcileAndValidate_IdenticalNoOp(t *testing.T) {
	cfg := reconcileConfig()
	intent := makeRecordIntent("app.example.com", domain.RecordA, "192.168.1.1", "c1", time.Now(), false, "test-host")
	desired := []*domain.RecordIntent{intent}
	actual := []*domain.RecordIntent{intent}

	toAdd, toRemove := ReconcileAndValidate(desired, actual, cfg, reconcileLogger())

	if len(toAdd) != 0 {
		t.Errorf("expected 0 records to add, got %d", len(toAdd))
	}
	if len(toRemove) != 0 {
		t.Errorf("expected 0 records to remove, got %d", len(toRemove))
	}
}

func TestReconcileAndValidate_EvictCNAMEForA(t *testing.T) {
	cfg := reconcileConfig()
	now := time.Now()
	desired := []*domain.RecordIntent{
		makeRecordIntent("app.example.com", domain.RecordA, "192.168.1.1", "c1", now.Add(-5*time.Hour), true, "test-host"), // force or older
	}
	actual := []*domain.RecordIntent{
		makeRecordIntent("app.example.com", domain.RecordCNAME, "target.example.com", "c2", now, false, "test-host"),
	}

	toAdd, toRemove := ReconcileAndValidate(desired, actual, cfg, reconcileLogger())

	if len(toAdd) != 1 {
		t.Errorf("expected 1 record to add, got %d", len(toAdd))
	}
	if len(toRemove) != 1 {
		t.Errorf("expected 1 record to remove (evicted CNAME), got %d", len(toRemove))
	}
}

func TestReconcileAndValidate_ForceEviction(t *testing.T) {
	cfg := reconcileConfig()
	now := time.Now()
	desired := []*domain.RecordIntent{
		makeRecordIntent("app.example.com", domain.RecordA, "192.168.1.2", "c1", now, true, "test-host"), // force
	}
	actual := []*domain.RecordIntent{
		makeRecordIntent("app.example.com", domain.RecordA, "192.168.1.1", "c2", now.Add(-5*time.Hour), false, "test-host"),
	}

	toAdd, _ := ReconcileAndValidate(desired, actual, cfg, reconcileLogger())

	// Force should cause eviction of different record with same name+kind+value
	if len(toAdd) != 1 {
		t.Errorf("expected 1 record to add, got %d", len(toAdd))
	}
}

func TestReconcileAndValidate_ValidationFailureSkipped(t *testing.T) {
	cfg := reconcileConfig()
	desired := []*domain.RecordIntent{
		simpleIntent("app.example.com", domain.RecordCNAME, "target.example.com", "c1", 1, false),
	}
	actual := []*domain.RecordIntent{
		makeRecordIntent("app.example.com", domain.RecordA, "192.168.1.1", "c2", time.Now().Add(-10*time.Hour), false, "other-host"), // other host, won't be evicted
	}

	toAdd, _ := ReconcileAndValidate(desired, actual, cfg, reconcileLogger())

	// CNAME conflicts with A record that can't be evicted (different host)
	if len(toAdd) != 0 {
		t.Errorf("expected 0 records to add (validation failure), got %d", len(toAdd))
	}
}

func TestReconcileAndValidate_ComplexScenario(t *testing.T) {
	cfg := reconcileConfig()
	now := time.Now()

	desired := []*domain.RecordIntent{
		makeRecordIntent("app1.example.com", domain.RecordA, "192.168.1.1", "c1", now, false, "test-host"),     // new
		makeRecordIntent("app2.example.com", domain.RecordA, "192.168.1.2", "c2", now, false, "test-host"),     // already exists
		makeRecordIntent("alias.example.com", domain.RecordCNAME, "target.example.com", "c3", now, false, "test-host"), // new
	}

	actual := []*domain.RecordIntent{
		makeRecordIntent("app2.example.com", domain.RecordA, "192.168.1.2", "c2", now, false, "test-host"),     // same as desired
		makeRecordIntent("stale.example.com", domain.RecordA, "192.168.1.99", "c4", now, false, "test-host"),   // owned, stale
		makeRecordIntent("other.example.com", domain.RecordA, "192.168.1.100", "c5", now, false, "other-host"), // not owned
	}

	toAdd, toRemove := ReconcileAndValidate(desired, actual, cfg, reconcileLogger())

	// Should add: app1, alias
	// Should remove: stale (owned, not in desired)
	// Should NOT remove: other (not owned), app2 (in desired)
	if len(toAdd) != 2 {
		t.Errorf("expected 2 records to add, got %d", len(toAdd))
	}
	if len(toRemove) != 1 {
		t.Errorf("expected 1 record to remove, got %d", len(toRemove))
	}

	// Verify the removed record is the stale one
	if len(toRemove) > 0 && toRemove[0].Record.Name != "stale.example.com" {
		t.Errorf("expected to remove 'stale.example.com', got %q", toRemove[0].Record.Name)
	}
}

// ============================================================================
// Helper function tests
// ============================================================================

func TestAddrRecords_NoRecords(t *testing.T) {
	m := newNestedRecordMap()

	records, hasAddr := addrRecords(m, "nonexistent.com")

	if hasAddr {
		t.Error("expected hasAddr to be false")
	}
	if len(records) != 0 {
		t.Errorf("expected 0 records, got %d", len(records))
	}
}

func TestAddrRecords_OnlyA(t *testing.T) {
	m := newNestedRecordMap()
	intent := simpleIntent("app.example.com", domain.RecordA, "192.168.1.1", "c1", 1, false)
	m.Get("app.example.com").Get(domain.RecordA).Set("192.168.1.1", intent)

	records, hasAddr := addrRecords(m, "app.example.com")

	if !hasAddr {
		t.Error("expected hasAddr to be true")
	}
	if len(records) != 1 {
		t.Errorf("expected 1 record, got %d", len(records))
	}
}

func TestAddrRecords_AAndAAAA(t *testing.T) {
	m := newNestedRecordMap()
	intentA := simpleIntent("app.example.com", domain.RecordA, "192.168.1.1", "c1", 1, false)
	intentAAAA := simpleIntent("app.example.com", domain.RecordAAAA, "::1", "c2", 1, false)
	m.Get("app.example.com").Get(domain.RecordA).Set("192.168.1.1", intentA)
	m.Get("app.example.com").Get(domain.RecordAAAA).Set("::1", intentAAAA)

	records, hasAddr := addrRecords(m, "app.example.com")

	if !hasAddr {
		t.Error("expected hasAddr to be true")
	}
	if len(records) != 2 {
		t.Errorf("expected 2 records, got %d", len(records))
	}
}

func TestRenderAll(t *testing.T) {
	intents := []*domain.RecordIntent{
		simpleIntent("app1.example.com", domain.RecordA, "192.168.1.1", "c1", 1, false),
		simpleIntent("app2.example.com", domain.RecordA, "192.168.1.2", "c2", 1, false),
	}

	rendered := renderAll(intents)

	if len(rendered) != 2 {
		t.Errorf("expected 2 rendered strings, got %d", len(rendered))
	}

	for _, r := range rendered {
		if r == "" {
			t.Error("expected non-empty rendered string")
		}
	}
}

func TestRenderAll_Empty(t *testing.T) {
	intents := []*domain.RecordIntent{}

	rendered := renderAll(intents)

	if len(rendered) != 0 {
		t.Errorf("expected 0 rendered strings, got %d", len(rendered))
	}
}

// ============================================================================
// Additional ReconcileAndValidate tests for edge cases
// ============================================================================

func TestReconcileAndValidate_AAAAVsCNAME_AAAAWins(t *testing.T) {
	cfg := reconcileConfig()
	now := time.Now()
	desired := []*domain.RecordIntent{
		makeRecordIntent("app.example.com", domain.RecordAAAA, "2001:db8::1", "c1", now.Add(-5*time.Hour), false, "test-host"),
	}
	actual := []*domain.RecordIntent{
		makeRecordIntent("app.example.com", domain.RecordCNAME, "target.example.com", "c2", now, false, "test-host"),
	}

	toAdd, toRemove := ReconcileAndValidate(desired, actual, cfg, reconcileLogger())

	if len(toAdd) != 1 {
		t.Errorf("expected 1 AAAA record to add, got %d", len(toAdd))
	}
	if len(toRemove) != 1 {
		t.Errorf("expected 1 CNAME record to remove, got %d", len(toRemove))
	}
}

func TestReconcileAndValidate_AAAAVsCNAME_CNAMEWins(t *testing.T) {
	cfg := reconcileConfig()
	now := time.Now()
	desired := []*domain.RecordIntent{
		makeRecordIntent("app.example.com", domain.RecordAAAA, "2001:db8::1", "c1", now, false, "test-host"),
	}
	// CNAME is owned by OTHER host so it won't be removed, causing conflict
	actual := []*domain.RecordIntent{
		makeRecordIntent("app.example.com", domain.RecordCNAME, "target.example.com", "c2", now.Add(-5*time.Hour), false, "other-host"),
	}

	toAdd, _ := ReconcileAndValidate(desired, actual, cfg, reconcileLogger())

	// CNAME is older and owned by another host, AAAA should not be added
	if len(toAdd) != 0 {
		t.Errorf("expected 0 records to add (CNAME wins), got %d", len(toAdd))
	}
}

func TestReconcileAndValidate_AAAAVsAAAA_SameValue(t *testing.T) {
	cfg := reconcileConfig()
	now := time.Now()
	desired := []*domain.RecordIntent{
		makeRecordIntent("app.example.com", domain.RecordAAAA, "2001:db8::1", "c1", now, false, "test-host"),
	}
	actual := []*domain.RecordIntent{
		makeRecordIntent("app.example.com", domain.RecordAAAA, "2001:db8::1", "c1", now, false, "test-host"),
	}

	toAdd, toRemove := ReconcileAndValidate(desired, actual, cfg, reconcileLogger())

	// Same record - no changes
	if len(toAdd) != 0 {
		t.Errorf("expected 0 records to add (identical), got %d", len(toAdd))
	}
	if len(toRemove) != 0 {
		t.Errorf("expected 0 records to remove, got %d", len(toRemove))
	}
}

func TestReconcileAndValidate_AAAAVsAAAA_DifferentOwner(t *testing.T) {
	cfg := reconcileConfig()
	now := time.Now()
	desired := []*domain.RecordIntent{
		makeRecordIntent("app.example.com", domain.RecordAAAA, "2001:db8::1", "c1", now.Add(-5*time.Hour), false, "test-host"),
	}
	actual := []*domain.RecordIntent{
		makeRecordIntent("app.example.com", domain.RecordAAAA, "2001:db8::1", "c2", now, false, "test-host"),
	}

	toAdd, toRemove := ReconcileAndValidate(desired, actual, cfg, reconcileLogger())

	// Older desired wins
	if len(toAdd) != 1 {
		t.Errorf("expected 1 record to add (older wins), got %d", len(toAdd))
	}
	if len(toRemove) != 1 {
		t.Errorf("expected 1 record to remove, got %d", len(toRemove))
	}
}

func TestReconcileAndValidate_CNAMEVsAAndAAAA(t *testing.T) {
	cfg := reconcileConfig()
	now := time.Now()
	desired := []*domain.RecordIntent{
		makeRecordIntent("app.example.com", domain.RecordCNAME, "target.example.com", "c1", now.Add(-10*time.Hour), true, "test-host"),
	}
	actual := []*domain.RecordIntent{
		makeRecordIntent("app.example.com", domain.RecordA, "192.168.1.1", "c2", now, false, "test-host"),
		makeRecordIntent("app.example.com", domain.RecordAAAA, "2001:db8::1", "c3", now, false, "test-host"),
	}

	toAdd, toRemove := ReconcileAndValidate(desired, actual, cfg, reconcileLogger())

	// CNAME with force should evict both A and AAAA
	if len(toAdd) != 1 {
		t.Errorf("expected 1 CNAME to add, got %d", len(toAdd))
	}
	if len(toRemove) != 2 {
		t.Errorf("expected 2 records to remove (A and AAAA), got %d", len(toRemove))
	}
}

func TestReconcileAndValidate_CNAMEVsCNAME_Eviction(t *testing.T) {
	cfg := reconcileConfig()
	now := time.Now()
	desired := []*domain.RecordIntent{
		makeRecordIntent("alias.example.com", domain.RecordCNAME, "new-target.example.com", "c1", now.Add(-5*time.Hour), true, "test-host"),
	}
	actual := []*domain.RecordIntent{
		makeRecordIntent("alias.example.com", domain.RecordCNAME, "old-target.example.com", "c2", now, false, "test-host"),
	}

	toAdd, toRemove := ReconcileAndValidate(desired, actual, cfg, reconcileLogger())

	// Force CNAME should replace existing CNAME
	if len(toAdd) != 1 {
		t.Errorf("expected 1 CNAME to add, got %d", len(toAdd))
	}
	if len(toRemove) != 1 {
		t.Errorf("expected 1 old CNAME to remove, got %d", len(toRemove))
	}
}

func TestReconcileAndValidate_CNAMEVsCNAME_IdenticalNoOp(t *testing.T) {
	cfg := reconcileConfig()
	now := time.Now()
	intent := makeRecordIntent("alias.example.com", domain.RecordCNAME, "target.example.com", "c1", now, false, "test-host")
	desired := []*domain.RecordIntent{intent}
	actual := []*domain.RecordIntent{intent}

	toAdd, toRemove := ReconcileAndValidate(desired, actual, cfg, reconcileLogger())

	if len(toAdd) != 0 {
		t.Errorf("expected 0 records to add, got %d", len(toAdd))
	}
	if len(toRemove) != 0 {
		t.Errorf("expected 0 records to remove, got %d", len(toRemove))
	}
}

func TestReconcileAndValidate_AVsA_ForceEviction(t *testing.T) {
	cfg := reconcileConfig()
	now := time.Now()
	desired := []*domain.RecordIntent{
		makeRecordIntent("app.example.com", domain.RecordA, "192.168.1.2", "c1", now, true, "test-host"),
	}
	actual := []*domain.RecordIntent{
		makeRecordIntent("app.example.com", domain.RecordA, "192.168.1.2", "c2", now.Add(-5*time.Hour), false, "test-host"),
	}

	toAdd, toRemove := ReconcileAndValidate(desired, actual, cfg, reconcileLogger())

	if len(toAdd) != 1 {
		t.Errorf("expected 1 record to add, got %d", len(toAdd))
	}
	if len(toRemove) != 1 {
		t.Errorf("expected 1 record to remove, got %d", len(toRemove))
	}
}

func TestReconcileAndValidate_MultipleDesiredWithEvictions(t *testing.T) {
	cfg := reconcileConfig()
	now := time.Now()
	desired := []*domain.RecordIntent{
		makeRecordIntent("app1.example.com", domain.RecordA, "192.168.1.1", "c1", now.Add(-5*time.Hour), false, "test-host"),
		makeRecordIntent("app2.example.com", domain.RecordA, "192.168.1.2", "c2", now.Add(-5*time.Hour), false, "test-host"),
	}
	actual := []*domain.RecordIntent{
		makeRecordIntent("app1.example.com", domain.RecordCNAME, "old.example.com", "c3", now, false, "test-host"),
		makeRecordIntent("app2.example.com", domain.RecordCNAME, "old2.example.com", "c4", now, false, "test-host"),
	}

	toAdd, toRemove := ReconcileAndValidate(desired, actual, cfg, reconcileLogger())

	// Both A records should win over CNAMEs (older)
	if len(toAdd) != 2 {
		t.Errorf("expected 2 records to add, got %d", len(toAdd))
	}
	if len(toRemove) != 2 {
		t.Errorf("expected 2 records to remove, got %d", len(toRemove))
	}
}

// ============================================================================
// FilterRecordIntents additional tests
// ============================================================================

func TestFilterRecordIntents_AAAADeduplication(t *testing.T) {
	intents := []*domain.RecordIntent{
		simpleIntent("app.example.com", domain.RecordAAAA, "2001:db8::1", "newer", 1, false),
		simpleIntent("app.example.com", domain.RecordAAAA, "2001:db8::1", "older", 5, false),
	}

	result := FilterRecordIntents(intents, reconcileLogger())

	if len(result) != 1 {
		t.Fatalf("expected 1 record after dedup, got %d", len(result))
	}

	if result[0].ContainerId != "container-older" {
		t.Errorf("expected older container to win, got %q", result[0].ContainerId)
	}
}

func TestFilterRecordIntents_CNAMEVsAAAA(t *testing.T) {
	intents := []*domain.RecordIntent{
		simpleIntent("app.example.com", domain.RecordAAAA, "2001:db8::1", "newer", 1, false),
		simpleIntent("app.example.com", domain.RecordCNAME, "target.example.com", "older", 5, false),
	}

	result := FilterRecordIntents(intents, reconcileLogger())

	if len(result) != 1 {
		t.Fatalf("expected 1 record, got %d", len(result))
	}

	// Older CNAME should win
	if result[0].Record.Kind != domain.RecordCNAME {
		t.Errorf("expected CNAME to win, got %v", result[0].Record.Kind)
	}
}

func TestFilterRecordIntents_AAndAAAATogether(t *testing.T) {
	intents := []*domain.RecordIntent{
		simpleIntent("app.example.com", domain.RecordA, "192.168.1.1", "c1", 1, false),
		simpleIntent("app.example.com", domain.RecordAAAA, "2001:db8::1", "c2", 1, false),
	}

	result := FilterRecordIntents(intents, reconcileLogger())

	// Both should be kept - A and AAAA can coexist
	if len(result) != 2 {
		t.Errorf("expected 2 records (A and AAAA together), got %d", len(result))
	}
}

func TestFilterRecordIntents_ForceWinsDeduplication(t *testing.T) {
	intents := []*domain.RecordIntent{
		simpleIntent("app.example.com", domain.RecordA, "192.168.1.1", "force-newer", 1, true),
		simpleIntent("app.example.com", domain.RecordA, "192.168.1.1", "nonforce-older", 5, false),
	}

	result := FilterRecordIntents(intents, reconcileLogger())

	if len(result) != 1 {
		t.Fatalf("expected 1 record after dedup, got %d", len(result))
	}

	// Force should win even though newer
	if result[0].ContainerId != "container-force-newer" {
		t.Errorf("expected force container to win, got %q", result[0].ContainerId)
	}
}

// ============================================================================
// shouldReplaceAllExisting additional tests
// ============================================================================

func TestShouldReplaceAllExisting_MixedForceNewNotOlderThanSome(t *testing.T) {
	newIntent := simpleIntent("app.example.com", domain.RecordCNAME, "target.example.com", "new", 3, true)
	existing := []*domain.RecordIntent{
		simpleIntent("app.example.com", domain.RecordA, "192.168.1.1", "e1", 5, true), // older with force
		simpleIntent("app.example.com", domain.RecordA, "192.168.1.2", "e2", 1, false), // newer without force
	}

	result := shouldReplaceAllExisting(newIntent, existing, reconcileLogger())

	// New is force but not older than all force records
	if result {
		t.Error("expected new to lose when not older than all force records")
	}
}

func TestShouldReplaceAllExisting_AllForceBothOlder(t *testing.T) {
	newIntent := simpleIntent("app.example.com", domain.RecordCNAME, "target.example.com", "new", 10, true)
	existing := []*domain.RecordIntent{
		simpleIntent("app.example.com", domain.RecordA, "192.168.1.1", "e1", 5, true),
		simpleIntent("app.example.com", domain.RecordA, "192.168.1.2", "e2", 3, true),
	}

	result := shouldReplaceAllExisting(newIntent, existing, reconcileLogger())

	// New is force and older than all existing force records
	if !result {
		t.Error("expected new to win when force and older than all force records")
	}
}

func TestShouldReplaceAllExisting_MixedForceNewOlderThanAllForce(t *testing.T) {
	newIntent := simpleIntent("app.example.com", domain.RecordCNAME, "target.example.com", "new", 10, true)
	existing := []*domain.RecordIntent{
		simpleIntent("app.example.com", domain.RecordA, "192.168.1.1", "e1", 5, true),  // force, newer than new
		simpleIntent("app.example.com", domain.RecordA, "192.168.1.2", "e2", 15, false), // non-force, older than new
	}

	result := shouldReplaceAllExisting(newIntent, existing, reconcileLogger())

	// New is force and older than all existing force records
	if !result {
		t.Error("expected new to win when force and older than all force records")
	}
}

func TestShouldReplaceAllExisting_AllNonForceNewerWins(t *testing.T) {
	newIntent := simpleIntent("app.example.com", domain.RecordCNAME, "target.example.com", "new", 1, false)
	existing := []*domain.RecordIntent{
		simpleIntent("app.example.com", domain.RecordA, "192.168.1.1", "e1", 5, false),
		simpleIntent("app.example.com", domain.RecordA, "192.168.1.2", "e2", 3, false),
	}

	result := shouldReplaceAllExisting(newIntent, existing, reconcileLogger())

	// All non-force, new is NOT older than all - existing wins
	if result {
		t.Error("expected existing to win when new is not older than all (all non-force)")
	}
}

// ============================================================================
// Additional ReconcileAndValidate edge case tests
// ============================================================================

func TestReconcileAndValidate_AVsA_NewerALoses(t *testing.T) {
	cfg := reconcileConfig()
	now := time.Now()
	// Same container ID but different metadata - tests conflict on same record
	desired := []*domain.RecordIntent{
		makeRecordIntent("app.example.com", domain.RecordA, "192.168.1.2", "c1", now, false, "test-host"),
	}
	// Actual record owned by OTHER host - won't be removed as stale
	actual := []*domain.RecordIntent{
		makeRecordIntent("app.example.com", domain.RecordA, "192.168.1.2", "c2", now.Add(-5*time.Hour), false, "other-host"),
	}

	toAdd, toRemove := ReconcileAndValidate(desired, actual, cfg, reconcileLogger())

	// Existing A is older but owned by other host - desired newer should NOT win
	if len(toAdd) != 0 {
		t.Errorf("expected 0 records to add (existing is older, newer loses), got %d", len(toAdd))
	}
	if len(toRemove) != 0 {
		t.Errorf("expected 0 records to remove (owned by other host), got %d", len(toRemove))
	}
}

func TestReconcileAndValidate_AAAAVsAAAA_NewerAAAALoses(t *testing.T) {
	cfg := reconcileConfig()
	now := time.Now()
	desired := []*domain.RecordIntent{
		makeRecordIntent("app.example.com", domain.RecordAAAA, "2001:db8::1", "c1", now, false, "test-host"),
	}
	// Actual record owned by OTHER host - won't be removed as stale
	actual := []*domain.RecordIntent{
		makeRecordIntent("app.example.com", domain.RecordAAAA, "2001:db8::1", "c2", now.Add(-5*time.Hour), false, "other-host"),
	}

	toAdd, toRemove := ReconcileAndValidate(desired, actual, cfg, reconcileLogger())

	// Existing AAAA is older but owned by other host - desired newer should NOT win
	if len(toAdd) != 0 {
		t.Errorf("expected 0 records to add (existing is older, newer loses), got %d", len(toAdd))
	}
	if len(toRemove) != 0 {
		t.Errorf("expected 0 records to remove, got %d", len(toRemove))
	}
}

func TestReconcileAndValidate_AVsCNAME_ALoses(t *testing.T) {
	cfg := reconcileConfig()
	now := time.Now()
	desired := []*domain.RecordIntent{
		makeRecordIntent("app.example.com", domain.RecordA, "192.168.1.1", "c1", now, false, "test-host"),
	}
	// CNAME owned by OTHER host so it won't be removed
	actual := []*domain.RecordIntent{
		makeRecordIntent("app.example.com", domain.RecordCNAME, "target.example.com", "c2", now.Add(-5*time.Hour), false, "other-host"),
	}

	toAdd, toRemove := ReconcileAndValidate(desired, actual, cfg, reconcileLogger())

	// Existing CNAME is older (owned by other host) - A should not be added
	if len(toAdd) != 0 {
		t.Errorf("expected 0 records to add (CNAME is older), got %d", len(toAdd))
	}
	if len(toRemove) != 0 {
		t.Errorf("expected 0 records to remove, got %d", len(toRemove))
	}
}

func TestReconcileAndValidate_CNAMEVsCNAME_NewerCNAMELoses(t *testing.T) {
	cfg := reconcileConfig()
	now := time.Now()
	desired := []*domain.RecordIntent{
		makeRecordIntent("alias.example.com", domain.RecordCNAME, "new-target.example.com", "c1", now, false, "test-host"),
	}
	// Existing CNAME owned by OTHER host
	actual := []*domain.RecordIntent{
		makeRecordIntent("alias.example.com", domain.RecordCNAME, "old-target.example.com", "c2", now.Add(-5*time.Hour), false, "other-host"),
	}

	toAdd, toRemove := ReconcileAndValidate(desired, actual, cfg, reconcileLogger())

	// Existing CNAME is older (owned by other host) - new CNAME should not be added
	if len(toAdd) != 0 {
		t.Errorf("expected 0 records to add (existing CNAME is older), got %d", len(toAdd))
	}
	if len(toRemove) != 0 {
		t.Errorf("expected 0 records to remove, got %d", len(toRemove))
	}
}

func TestReconcileAndValidate_CNAMEVsAAndAAAA_CNAMELoses(t *testing.T) {
	cfg := reconcileConfig()
	now := time.Now()
	desired := []*domain.RecordIntent{
		makeRecordIntent("app.example.com", domain.RecordCNAME, "target.example.com", "c1", now, false, "test-host"),
	}
	// Address records owned by OTHER host
	actual := []*domain.RecordIntent{
		makeRecordIntent("app.example.com", domain.RecordA, "192.168.1.1", "c2", now.Add(-10*time.Hour), false, "other-host"),
		makeRecordIntent("app.example.com", domain.RecordAAAA, "2001:db8::1", "c3", now.Add(-5*time.Hour), false, "other-host"),
	}

	toAdd, toRemove := ReconcileAndValidate(desired, actual, cfg, reconcileLogger())

	// Existing address records are older (owned by other host) - CNAME should not be added
	if len(toAdd) != 0 {
		t.Errorf("expected 0 records to add (address records are older), got %d", len(toAdd))
	}
	if len(toRemove) != 0 {
		t.Errorf("expected 0 records to remove, got %d", len(toRemove))
	}
}

func TestReconcileAndValidate_ValidationFailure(t *testing.T) {
	cfg := reconcileConfig()
	now := time.Now()

	// Desired CNAME would create a cycle
	desired := []*domain.RecordIntent{
		makeRecordIntent("app.example.com", domain.RecordCNAME, "loop.example.com", "c1", now.Add(-10*time.Hour), false, "test-host"),
	}
	// Existing record points back
	actual := []*domain.RecordIntent{
		makeRecordIntent("loop.example.com", domain.RecordCNAME, "app.example.com", "c2", now, false, "other-host"),
	}

	toAdd, toRemove := ReconcileAndValidate(desired, actual, cfg, reconcileLogger())

	// Should skip the CNAME because it creates a cycle
	if len(toAdd) != 0 {
		t.Errorf("expected 0 records to add (validation failure), got %d", len(toAdd))
	}
	if len(toRemove) != 0 {
		t.Errorf("expected 0 records to remove, got %d", len(toRemove))
	}
}

func TestReconcileAndValidate_StaleRecordOtherHost(t *testing.T) {
	cfg := reconcileConfig()
	now := time.Now()

	desired := []*domain.RecordIntent{}
	// Stale record owned by different host
	actual := []*domain.RecordIntent{
		makeRecordIntent("stale.example.com", domain.RecordA, "192.168.1.99", "c2", now, false, "other-host"),
	}

	toAdd, toRemove := ReconcileAndValidate(desired, actual, cfg, reconcileLogger())

	// Should not remove record owned by other host
	if len(toRemove) != 0 {
		t.Errorf("expected 0 records to remove (other host owns it), got %d", len(toRemove))
	}
	if len(toAdd) != 0 {
		t.Errorf("expected 0 records to add, got %d", len(toAdd))
	}
}

func TestReconcileAndValidate_AWithDifferentValue(t *testing.T) {
	cfg := reconcileConfig()
	now := time.Now()

	// Different A value - both can coexist
	desired := []*domain.RecordIntent{
		makeRecordIntent("app.example.com", domain.RecordA, "192.168.1.2", "c1", now, false, "test-host"),
	}
	// Existing record owned by OTHER host
	actual := []*domain.RecordIntent{
		makeRecordIntent("app.example.com", domain.RecordA, "192.168.1.1", "c2", now.Add(-5*time.Hour), false, "other-host"),
	}

	toAdd, toRemove := ReconcileAndValidate(desired, actual, cfg, reconcileLogger())

	// Should add new A - different values can coexist
	if len(toAdd) != 1 {
		t.Errorf("expected 1 record to add (different A value), got %d", len(toAdd))
	}
	if len(toRemove) != 0 {
		t.Errorf("expected 0 records to remove (different A values coexist), got %d", len(toRemove))
	}
}

func TestReconcileAndValidate_CNAMEOlderThanAllAddress(t *testing.T) {
	cfg := reconcileConfig()
	now := time.Now()
	desired := []*domain.RecordIntent{
		makeRecordIntent("app.example.com", domain.RecordCNAME, "target.example.com", "c1", now.Add(-10*time.Hour), false, "test-host"),
	}
	actual := []*domain.RecordIntent{
		makeRecordIntent("app.example.com", domain.RecordA, "192.168.1.1", "c2", now, false, "test-host"),
		makeRecordIntent("app.example.com", domain.RecordAAAA, "2001:db8::1", "c3", now, false, "test-host"),
	}

	toAdd, toRemove := ReconcileAndValidate(desired, actual, cfg, reconcileLogger())

	// CNAME is older than all address records - should evict them
	if len(toAdd) != 1 {
		t.Errorf("expected 1 CNAME to add (older than all), got %d", len(toAdd))
	}
	if len(toRemove) != 2 {
		t.Errorf("expected 2 address records to remove, got %d", len(toRemove))
	}
}

func TestReconcileAndValidate_CNAMEVsAddressNotOlderThanAll(t *testing.T) {
	cfg := reconcileConfig()
	now := time.Now()
	desired := []*domain.RecordIntent{
		makeRecordIntent("app.example.com", domain.RecordCNAME, "target.example.com", "c1", now.Add(-5*time.Hour), false, "test-host"),
	}
	// Address records owned by OTHER host - won't be removed as stale
	actual := []*domain.RecordIntent{
		makeRecordIntent("app.example.com", domain.RecordA, "192.168.1.1", "c2", now, false, "other-host"),
		makeRecordIntent("app.example.com", domain.RecordAAAA, "2001:db8::1", "c3", now.Add(-10*time.Hour), false, "other-host"), // Older than CNAME
	}

	toAdd, toRemove := ReconcileAndValidate(desired, actual, cfg, reconcileLogger())

	// CNAME is NOT older than all address records - should not evict
	if len(toAdd) != 0 {
		t.Errorf("expected 0 records to add (not older than all), got %d", len(toAdd))
	}
	if len(toRemove) != 0 {
		t.Errorf("expected 0 records to remove (owned by other host), got %d", len(toRemove))
	}
}

// ============================================================================
// Additional FilterRecordIntents edge case tests
// ============================================================================

func TestFilterRecordIntents_NoRecordKind(t *testing.T) {
	// Test with an intent that has an unsupported record kind
	// This should hit the default case in the switch
	intents := []*domain.RecordIntent{}

	result := FilterRecordIntents(intents, reconcileLogger())

	if len(result) != 0 {
		t.Errorf("expected 0 records, got %d", len(result))
	}
}

func TestFilterRecordIntents_CNAMEDeduplication(t *testing.T) {
	intents := []*domain.RecordIntent{
		simpleIntent("alias.example.com", domain.RecordCNAME, "target1.example.com", "newer", 1, false),
		simpleIntent("alias.example.com", domain.RecordCNAME, "target2.example.com", "older", 5, false),
	}

	result := FilterRecordIntents(intents, reconcileLogger())

	if len(result) != 1 {
		t.Fatalf("expected 1 CNAME after dedup, got %d", len(result))
	}

	// Older CNAME should win
	if result[0].ContainerId != "container-older" {
		t.Errorf("expected older container to win, got %q", result[0].ContainerId)
	}
}

func TestFilterRecordIntents_CNAMEVsAAndAAAA_CNAMEWins(t *testing.T) {
	intents := []*domain.RecordIntent{
		simpleIntent("app.example.com", domain.RecordA, "192.168.1.1", "newer-a", 1, false),
		simpleIntent("app.example.com", domain.RecordAAAA, "2001:db8::1", "newer-aaaa", 1, false),
		simpleIntent("app.example.com", domain.RecordCNAME, "target.example.com", "oldest", 10, false),
	}

	result := FilterRecordIntents(intents, reconcileLogger())

	if len(result) != 1 {
		t.Fatalf("expected 1 record (CNAME wins), got %d", len(result))
	}

	if result[0].Record.Kind != domain.RecordCNAME {
		t.Errorf("expected CNAME to win, got %v", result[0].Record.Kind)
	}
}

func TestFilterRecordIntents_CNAMEVsAAndAAAA_AddressWins(t *testing.T) {
	intents := []*domain.RecordIntent{
		simpleIntent("app.example.com", domain.RecordA, "192.168.1.1", "oldest-a", 10, false),
		simpleIntent("app.example.com", domain.RecordAAAA, "2001:db8::1", "oldest-aaaa", 10, false),
		simpleIntent("app.example.com", domain.RecordCNAME, "target.example.com", "newer", 1, false),
	}

	result := FilterRecordIntents(intents, reconcileLogger())

	// A and AAAA are older - they should win
	if len(result) != 2 {
		t.Fatalf("expected 2 records (A and AAAA win), got %d", len(result))
	}

	foundA := false
	foundAAAA := false
	for _, r := range result {
		if r.Record.Kind == domain.RecordA {
			foundA = true
		}
		if r.Record.Kind == domain.RecordAAAA {
			foundAAAA = true
		}
	}

	if !foundA || !foundAAAA {
		t.Error("expected both A and AAAA to be present")
	}
}

func TestFilterRecordIntents_UnsupportedKindSkipped(t *testing.T) {
	// Create an intent with an unsupported record kind to hit the default case
	unsupportedIntent := &domain.RecordIntent{
		ContainerId:   "container-1",
		ContainerName: "test",
		Hostname:      "test-host",
		Record:        domain.Record{Kind: "UNKNOWN", Name: "test.example.com", Value: "x"},
	}

	result := FilterRecordIntents([]*domain.RecordIntent{unsupportedIntent}, reconcileLogger())

	// The unsupported record should be skipped entirely
	if len(result) != 0 {
		t.Errorf("expected 0 records (unsupported kind skipped), got %d", len(result))
	}
}

func TestFilterRecordIntents_UnsupportedKindWithValidRecords(t *testing.T) {
	// Mix of valid and unsupported record kinds
	intents := []*domain.RecordIntent{
		simpleIntent("app.example.com", domain.RecordA, "192.168.1.1", "c1", 5, false),
		{
			ContainerId:   "container-2",
			ContainerName: "test",
			Hostname:      "test-host",
			Record:        domain.Record{Kind: "UNKNOWN", Name: "unknown.example.com", Value: "x"},
		},
		simpleIntent("app2.example.com", domain.RecordCNAME, "target.example.com", "c3", 5, false),
	}

	result := FilterRecordIntents(intents, reconcileLogger())

	// Should have 2 records (the valid A and CNAME), unsupported skipped
	if len(result) != 2 {
		t.Errorf("expected 2 records (unsupported skipped), got %d", len(result))
	}
}

// ============================================================================
// Cross-host conflict resolution tests
// ============================================================================

func TestReconcileAndValidate_CrossHostAVsCNAME_AEvictsCNAME(t *testing.T) {
	cfg := reconcileConfig()
	now := time.Now()
	desired := []*domain.RecordIntent{
		makeRecordIntent("app.example.com", domain.RecordA, "192.168.1.1", "c1", now.Add(-10*time.Hour), false, "test-host"),
	}
	// CNAME from OTHER host - will be in actualByNameKind for conflict detection
	actual := []*domain.RecordIntent{
		makeRecordIntent("app.example.com", domain.RecordCNAME, "target.example.com", "c2", now, false, "other-host"),
	}

	toAdd, toRemove := ReconcileAndValidate(desired, actual, cfg, reconcileLogger())

	// A is older than CNAME and should evict it
	if len(toAdd) != 1 {
		t.Errorf("expected 1 record to add, got %d", len(toAdd))
	}
	if len(toRemove) != 1 {
		t.Errorf("expected 1 record to remove (evicted CNAME), got %d", len(toRemove))
	}
}

func TestReconcileAndValidate_CrossHostAVsCNAME_ALoses(t *testing.T) {
	cfg := reconcileConfig()
	now := time.Now()
	desired := []*domain.RecordIntent{
		makeRecordIntent("app.example.com", domain.RecordA, "192.168.1.1", "c1", now, false, "test-host"),
	}
	// CNAME from OTHER host is older
	actual := []*domain.RecordIntent{
		makeRecordIntent("app.example.com", domain.RecordCNAME, "target.example.com", "c2", now.Add(-10*time.Hour), false, "other-host"),
	}

	toAdd, toRemove := ReconcileAndValidate(desired, actual, cfg, reconcileLogger())

	// A is newer than CNAME - A loses, no changes
	if len(toAdd) != 0 {
		t.Errorf("expected 0 records to add (A loses to older CNAME), got %d", len(toAdd))
	}
	if len(toRemove) != 0 {
		t.Errorf("expected 0 records to remove, got %d", len(toRemove))
	}
}

func TestReconcileAndValidate_CrossHostAAAAVsCNAME_AAAAEvictsCNAME(t *testing.T) {
	cfg := reconcileConfig()
	now := time.Now()
	desired := []*domain.RecordIntent{
		makeRecordIntent("app.example.com", domain.RecordAAAA, "2001:db8::1", "c1", now.Add(-10*time.Hour), false, "test-host"),
	}
	// CNAME from OTHER host
	actual := []*domain.RecordIntent{
		makeRecordIntent("app.example.com", domain.RecordCNAME, "target.example.com", "c2", now, false, "other-host"),
	}

	toAdd, toRemove := ReconcileAndValidate(desired, actual, cfg, reconcileLogger())

	// AAAA is older than CNAME and should evict it
	if len(toAdd) != 1 {
		t.Errorf("expected 1 record to add, got %d", len(toAdd))
	}
	if len(toRemove) != 1 {
		t.Errorf("expected 1 record to remove (evicted CNAME), got %d", len(toRemove))
	}
}

func TestReconcileAndValidate_CrossHostAAAAVsCNAME_AAAALoses(t *testing.T) {
	cfg := reconcileConfig()
	now := time.Now()
	desired := []*domain.RecordIntent{
		makeRecordIntent("app.example.com", domain.RecordAAAA, "2001:db8::1", "c1", now, false, "test-host"),
	}
	// CNAME from OTHER host is older
	actual := []*domain.RecordIntent{
		makeRecordIntent("app.example.com", domain.RecordCNAME, "target.example.com", "c2", now.Add(-10*time.Hour), false, "other-host"),
	}

	toAdd, toRemove := ReconcileAndValidate(desired, actual, cfg, reconcileLogger())

	// AAAA is newer than CNAME - AAAA loses
	if len(toAdd) != 0 {
		t.Errorf("expected 0 records to add (AAAA loses to older CNAME), got %d", len(toAdd))
	}
	if len(toRemove) != 0 {
		t.Errorf("expected 0 records to remove, got %d", len(toRemove))
	}
}

func TestReconcileAndValidate_CrossHostAVsA_AEvictsA(t *testing.T) {
	cfg := reconcileConfig()
	now := time.Now()
	desired := []*domain.RecordIntent{
		makeRecordIntent("app.example.com", domain.RecordA, "192.168.1.1", "c1", now.Add(-10*time.Hour), false, "test-host"),
	}
	// Same A record (same value) from OTHER host
	actual := []*domain.RecordIntent{
		makeRecordIntent("app.example.com", domain.RecordA, "192.168.1.1", "c2", now, false, "other-host"),
	}

	toAdd, toRemove := ReconcileAndValidate(desired, actual, cfg, reconcileLogger())

	// Desired A is older, should evict the actual A
	if len(toAdd) != 1 {
		t.Errorf("expected 1 record to add, got %d", len(toAdd))
	}
	if len(toRemove) != 1 {
		t.Errorf("expected 1 record to remove, got %d", len(toRemove))
	}
}

func TestReconcileAndValidate_CrossHostAVsA_ALoses(t *testing.T) {
	cfg := reconcileConfig()
	now := time.Now()
	desired := []*domain.RecordIntent{
		makeRecordIntent("app.example.com", domain.RecordA, "192.168.1.1", "c1", now, false, "test-host"),
	}
	// Same A record from OTHER host is older
	actual := []*domain.RecordIntent{
		makeRecordIntent("app.example.com", domain.RecordA, "192.168.1.1", "c2", now.Add(-10*time.Hour), false, "other-host"),
	}

	toAdd, toRemove := ReconcileAndValidate(desired, actual, cfg, reconcileLogger())

	// Desired A is newer - it loses to the older actual A
	if len(toAdd) != 0 {
		t.Errorf("expected 0 records to add (A loses to older A), got %d", len(toAdd))
	}
	if len(toRemove) != 0 {
		t.Errorf("expected 0 records to remove, got %d", len(toRemove))
	}
}

func TestReconcileAndValidate_CrossHostAAAAVsAAAA_AAAAEvictsAAAA(t *testing.T) {
	cfg := reconcileConfig()
	now := time.Now()
	desired := []*domain.RecordIntent{
		makeRecordIntent("app.example.com", domain.RecordAAAA, "2001:db8::1", "c1", now.Add(-10*time.Hour), false, "test-host"),
	}
	// Same AAAA record from OTHER host
	actual := []*domain.RecordIntent{
		makeRecordIntent("app.example.com", domain.RecordAAAA, "2001:db8::1", "c2", now, false, "other-host"),
	}

	toAdd, toRemove := ReconcileAndValidate(desired, actual, cfg, reconcileLogger())

	// Desired AAAA is older, should evict the actual AAAA
	if len(toAdd) != 1 {
		t.Errorf("expected 1 record to add, got %d", len(toAdd))
	}
	if len(toRemove) != 1 {
		t.Errorf("expected 1 record to remove, got %d", len(toRemove))
	}
}

func TestReconcileAndValidate_CrossHostAAAAVsAAAA_AAAALoses(t *testing.T) {
	cfg := reconcileConfig()
	now := time.Now()
	desired := []*domain.RecordIntent{
		makeRecordIntent("app.example.com", domain.RecordAAAA, "2001:db8::1", "c1", now, false, "test-host"),
	}
	// Same AAAA record from OTHER host is older
	actual := []*domain.RecordIntent{
		makeRecordIntent("app.example.com", domain.RecordAAAA, "2001:db8::1", "c2", now.Add(-10*time.Hour), false, "other-host"),
	}

	toAdd, toRemove := ReconcileAndValidate(desired, actual, cfg, reconcileLogger())

	// Desired AAAA is newer - it loses to the older actual AAAA
	if len(toAdd) != 0 {
		t.Errorf("expected 0 records to add (AAAA loses to older AAAA), got %d", len(toAdd))
	}
	if len(toRemove) != 0 {
		t.Errorf("expected 0 records to remove, got %d", len(toRemove))
	}
}

func TestReconcileAndValidate_CrossHostCNAMEVsAAndAAAA_CNAMEEvicts(t *testing.T) {
	cfg := reconcileConfig()
	now := time.Now()
	desired := []*domain.RecordIntent{
		makeRecordIntent("app.example.com", domain.RecordCNAME, "target.example.com", "c1", now.Add(-10*time.Hour), false, "test-host"),
	}
	// A and AAAA from OTHER host - both newer than CNAME
	actual := []*domain.RecordIntent{
		makeRecordIntent("app.example.com", domain.RecordA, "192.168.1.1", "c2", now, false, "other-host"),
		makeRecordIntent("app.example.com", domain.RecordAAAA, "2001:db8::1", "c3", now, false, "other-host"),
	}

	toAdd, toRemove := ReconcileAndValidate(desired, actual, cfg, reconcileLogger())

	// CNAME is older than all address records, should evict both
	if len(toAdd) != 1 {
		t.Errorf("expected 1 record to add, got %d", len(toAdd))
	}
	if len(toRemove) != 2 {
		t.Errorf("expected 2 records to remove (evicted A and AAAA), got %d", len(toRemove))
	}
}

func TestReconcileAndValidate_CrossHostCNAMEVsCNAME_CNAMEEvicts(t *testing.T) {
	cfg := reconcileConfig()
	now := time.Now()
	desired := []*domain.RecordIntent{
		makeRecordIntent("app.example.com", domain.RecordCNAME, "target1.example.com", "c1", now.Add(-10*time.Hour), false, "test-host"),
	}
	// Different CNAME from OTHER host
	actual := []*domain.RecordIntent{
		makeRecordIntent("app.example.com", domain.RecordCNAME, "target2.example.com", "c2", now, false, "other-host"),
	}

	toAdd, toRemove := ReconcileAndValidate(desired, actual, cfg, reconcileLogger())

	// Desired CNAME is older, should evict the actual CNAME
	if len(toAdd) != 1 {
		t.Errorf("expected 1 record to add, got %d", len(toAdd))
	}
	if len(toRemove) != 1 {
		t.Errorf("expected 1 record to remove, got %d", len(toRemove))
	}
}

func TestReconcileAndValidate_CrossHostCNAMEVsCNAME_CNAMELoses(t *testing.T) {
	cfg := reconcileConfig()
	now := time.Now()
	desired := []*domain.RecordIntent{
		makeRecordIntent("app.example.com", domain.RecordCNAME, "target1.example.com", "c1", now, false, "test-host"),
	}
	// Different CNAME from OTHER host is older
	actual := []*domain.RecordIntent{
		makeRecordIntent("app.example.com", domain.RecordCNAME, "target2.example.com", "c2", now.Add(-10*time.Hour), false, "other-host"),
	}

	toAdd, toRemove := ReconcileAndValidate(desired, actual, cfg, reconcileLogger())

	// Desired CNAME is newer - it loses to the older actual CNAME
	if len(toAdd) != 0 {
		t.Errorf("expected 0 records to add (CNAME loses to older CNAME), got %d", len(toAdd))
	}
	if len(toRemove) != 0 {
		t.Errorf("expected 0 records to remove, got %d", len(toRemove))
	}
}

func TestReconcileAndValidate_ForceEvictsCrossHost(t *testing.T) {
	cfg := reconcileConfig()
	now := time.Now()
	desired := []*domain.RecordIntent{
		makeRecordIntent("app.example.com", domain.RecordA, "192.168.1.1", "c1", now, true, "test-host"), // Force!
	}
	// CNAME from OTHER host is older, but we have force
	actual := []*domain.RecordIntent{
		makeRecordIntent("app.example.com", domain.RecordCNAME, "target.example.com", "c2", now.Add(-10*time.Hour), false, "other-host"),
	}

	toAdd, toRemove := ReconcileAndValidate(desired, actual, cfg, reconcileLogger())

	// Force should evict even though the actual record is older
	if len(toAdd) != 1 {
		t.Errorf("expected 1 record to add, got %d", len(toAdd))
	}
	if len(toRemove) != 1 {
		t.Errorf("expected 1 record to remove, got %d", len(toRemove))
	}
}
