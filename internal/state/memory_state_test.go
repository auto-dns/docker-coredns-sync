package state

import (
	"sync"
	"testing"
	"time"

	"github.com/auto-dns/docker-coredns-sync/internal/domain"
)

func makeTestIntent(name, value string) *domain.RecordIntent {
	rec, _ := domain.NewA(name, value)
	return &domain.RecordIntent{
		ContainerId:   "test-container",
		ContainerName: "test",
		Created:       time.Now(),
		Hostname:      "test-host",
		Force:         false,
		Record:        rec,
	}
}

func TestMemoryState_Upsert_NewContainer(t *testing.T) {
	state := NewMemoryState()
	intents := []*domain.RecordIntent{
		makeTestIntent("app.example.com", "192.168.1.1"),
	}
	created := time.Now()

	state.Upsert("container-1", "my-app", created, intents, "running")

	// Verify by getting intents
	result := state.GetAllDesiredRecordIntents()
	if len(result) != 1 {
		t.Errorf("expected 1 intent, got %d", len(result))
	}
}

func TestMemoryState_Upsert_UpdateExisting(t *testing.T) {
	state := NewMemoryState()
	created := time.Now()

	// Initial insert
	intents1 := []*domain.RecordIntent{
		makeTestIntent("app1.example.com", "192.168.1.1"),
	}
	state.Upsert("container-1", "my-app", created, intents1, "running")

	// Update with new intents
	intents2 := []*domain.RecordIntent{
		makeTestIntent("app2.example.com", "192.168.1.2"),
		makeTestIntent("app3.example.com", "192.168.1.3"),
	}
	state.Upsert("container-1", "my-app-updated", created, intents2, "running")

	result := state.GetAllDesiredRecordIntents()
	if len(result) != 2 {
		t.Errorf("expected 2 intents after update, got %d", len(result))
	}
}

func TestMemoryState_MarkRemoved_Exists(t *testing.T) {
	state := NewMemoryState()
	intents := []*domain.RecordIntent{
		makeTestIntent("app.example.com", "192.168.1.1"),
	}
	state.Upsert("container-1", "my-app", time.Now(), intents, "running")

	removed := state.MarkRemoved("container-1")

	if !removed {
		t.Error("expected MarkRemoved to return true for existing container")
	}

	// Verify intents are no longer returned
	result := state.GetAllDesiredRecordIntents()
	if len(result) != 0 {
		t.Errorf("expected 0 intents after removal, got %d", len(result))
	}
}

func TestMemoryState_RetainRunning(t *testing.T) {
	state := NewMemoryState()
	state.Upsert("keep", "keep-app", time.Now(), []*domain.RecordIntent{
		makeTestIntent("keep.example.com", "192.168.1.1"),
	}, "running")
	state.Upsert("drop", "drop-app", time.Now(), []*domain.RecordIntent{
		makeTestIntent("drop.example.com", "192.168.1.2"),
	}, "running")

	removed := state.RetainRunning(map[string]struct{}{"keep": {}})

	if removed != 1 {
		t.Errorf("expected 1 container pruned, got %d", removed)
	}

	result := state.GetAllDesiredRecordIntents()
	if len(result) != 1 {
		t.Fatalf("expected 1 desired intent after prune, got %d", len(result))
	}
	if result[0].Record.Name != "keep.example.com" {
		t.Errorf("expected the kept container's record, got %q", result[0].Record.Name)
	}
}

func TestMemoryState_RetainRunning_AlreadyRemovedUnaffected(t *testing.T) {
	state := NewMemoryState()
	state.Upsert("c1", "app", time.Now(), []*domain.RecordIntent{
		makeTestIntent("app.example.com", "192.168.1.1"),
	}, "running")
	state.MarkRemoved("c1")

	// c1 is already removed; an empty running set should prune nothing new.
	if removed := state.RetainRunning(map[string]struct{}{}); removed != 0 {
		t.Errorf("expected 0 newly pruned, got %d", removed)
	}
}

func TestMemoryState_MarkRemoved_NotExists(t *testing.T) {
	state := NewMemoryState()

	removed := state.MarkRemoved("nonexistent")

	if removed {
		t.Error("expected MarkRemoved to return false for nonexistent container")
	}
}

func TestMemoryState_GetAllDesiredRecordIntents_Running(t *testing.T) {
	state := NewMemoryState()

	state.Upsert("container-1", "app1", time.Now(), []*domain.RecordIntent{
		makeTestIntent("app1.example.com", "192.168.1.1"),
	}, "running")

	state.Upsert("container-2", "app2", time.Now(), []*domain.RecordIntent{
		makeTestIntent("app2.example.com", "192.168.1.2"),
	}, "running")

	result := state.GetAllDesiredRecordIntents()

	if len(result) != 2 {
		t.Errorf("expected 2 intents from running containers, got %d", len(result))
	}
}

func TestMemoryState_GetAllDesiredRecordIntents_IgnoresRemoved(t *testing.T) {
	state := NewMemoryState()

	state.Upsert("container-1", "app1", time.Now(), []*domain.RecordIntent{
		makeTestIntent("app1.example.com", "192.168.1.1"),
	}, "running")

	state.Upsert("container-2", "app2", time.Now(), []*domain.RecordIntent{
		makeTestIntent("app2.example.com", "192.168.1.2"),
	}, "running")

	state.MarkRemoved("container-1")

	result := state.GetAllDesiredRecordIntents()

	if len(result) != 1 {
		t.Errorf("expected 1 intent (removed excluded), got %d", len(result))
	}

	if result[0].Record.Name != "app2.example.com" {
		t.Errorf("expected app2.example.com, got %q", result[0].Record.Name)
	}
}

func TestMemoryState_GetAllDesiredRecordIntents_Empty(t *testing.T) {
	state := NewMemoryState()

	result := state.GetAllDesiredRecordIntents()

	if len(result) != 0 {
		t.Errorf("expected 0 intents for empty state, got %d", len(result))
	}
}

func TestMemoryState_GetAllDesiredRecordIntents_MultipleIntentsPerContainer(t *testing.T) {
	state := NewMemoryState()

	state.Upsert("container-1", "app1", time.Now(), []*domain.RecordIntent{
		makeTestIntent("web.example.com", "192.168.1.1"),
		makeTestIntent("api.example.com", "192.168.1.2"),
		makeTestIntent("db.example.com", "192.168.1.3"),
	}, "running")

	result := state.GetAllDesiredRecordIntents()

	if len(result) != 3 {
		t.Errorf("expected 3 intents, got %d", len(result))
	}
}

func TestMemoryState_ConcurrentAccess(t *testing.T) {
	state := NewMemoryState()
	var wg sync.WaitGroup
	numGoroutines := 100

	// Concurrent writes
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			containerId := "container-" + string(rune('A'+id%26))
			state.Upsert(containerId, "app", time.Now(), []*domain.RecordIntent{
				makeTestIntent("app.example.com", "192.168.1.1"),
			}, "running")
		}(i)
	}

	// Concurrent reads
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = state.GetAllDesiredRecordIntents()
		}()
	}

	// Concurrent mark removed
	for i := 0; i < numGoroutines/2; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			containerId := "container-" + string(rune('A'+id%26))
			state.MarkRemoved(containerId)
		}(i)
	}

	wg.Wait()

	// Should not panic or deadlock
}

func TestMemoryState_UpsertUpdatesLastUpdated(t *testing.T) {
	state := NewMemoryState()
	created := time.Now().Add(-time.Hour)

	state.Upsert("container-1", "app", created, []*domain.RecordIntent{}, "running")

	// The LastUpdated should be set to approximately now, not the created time
	// We can't directly check this without exposing internals, but we can verify
	// that subsequent operations work correctly
	state.Upsert("container-1", "app-updated", created, []*domain.RecordIntent{
		makeTestIntent("app.example.com", "192.168.1.1"),
	}, "running")

	result := state.GetAllDesiredRecordIntents()
	if len(result) != 1 {
		t.Errorf("expected 1 intent after update, got %d", len(result))
	}
}

func TestMemoryState_NonRunningStatusIgnored(t *testing.T) {
	state := NewMemoryState()

	// Directly upsert with non-running status
	state.Upsert("container-1", "app", time.Now(), []*domain.RecordIntent{
		makeTestIntent("app.example.com", "192.168.1.1"),
	}, "stopped")

	result := state.GetAllDesiredRecordIntents()

	if len(result) != 0 {
		t.Errorf("expected 0 intents for non-running container, got %d", len(result))
	}
}

func TestMemoryState_ReaddAfterRemove(t *testing.T) {
	state := NewMemoryState()

	// Add
	state.Upsert("container-1", "app", time.Now(), []*domain.RecordIntent{
		makeTestIntent("app.example.com", "192.168.1.1"),
	}, "running")

	// Remove
	state.MarkRemoved("container-1")

	// Re-add
	state.Upsert("container-1", "app", time.Now(), []*domain.RecordIntent{
		makeTestIntent("app.example.com", "192.168.1.2"),
	}, "running")

	result := state.GetAllDesiredRecordIntents()

	if len(result) != 1 {
		t.Errorf("expected 1 intent after re-add, got %d", len(result))
	}

	if result[0].Record.Value != "192.168.1.2" {
		t.Errorf("expected new value '192.168.1.2', got %q", result[0].Record.Value)
	}
}
