package httpserver

import (
	"errors"
	"testing"
	"time"
)

func TestStatus_Ready_NotConnected(t *testing.T) {
	s := NewStatus(time.Minute)
	s.RecordReconcile(nil) // successful reconcile, but docker not connected
	if ready, reason := s.Ready(); ready {
		t.Errorf("expected not ready when docker disconnected, got ready (reason=%q)", reason)
	}
}

func TestStatus_Ready_NoReconcileYet(t *testing.T) {
	s := NewStatus(time.Minute)
	s.SetDockerConnected(true)
	if ready, reason := s.Ready(); ready {
		t.Errorf("expected not ready before first reconcile, got ready (reason=%q)", reason)
	}
}

func TestStatus_Ready_Healthy(t *testing.T) {
	s := NewStatus(time.Minute)
	s.SetDockerConnected(true)
	s.RecordReconcile(nil)
	if ready, reason := s.Ready(); !ready {
		t.Errorf("expected ready, got not ready: %s", reason)
	}
}

func TestStatus_Ready_StaleReconcile(t *testing.T) {
	s := NewStatus(time.Minute)
	now := time.Now()
	s.now = func() time.Time { return now }
	s.SetDockerConnected(true)
	s.RecordReconcile(nil)

	// Advance time beyond the threshold.
	now = now.Add(2 * time.Minute)
	if ready, _ := s.Ready(); ready {
		t.Error("expected not ready when last reconcile is stale")
	}
}

func TestStatus_Ready_DryRunNeverReady(t *testing.T) {
	s := NewStatus(time.Minute)
	s.SetDryRun(true)
	s.SetDockerConnected(true)
	s.RecordReconcile(nil) // even a "successful" pass must not flip to ready

	if ready, reason := s.Ready(); ready {
		t.Errorf("expected dry-run to never be ready, got ready (reason=%q)", reason)
	}
}

func TestStatus_RecordReconcile_ErrorDoesNotRefresh(t *testing.T) {
	s := NewStatus(time.Minute)
	s.SetDockerConnected(true)
	s.RecordReconcile(errors.New("boom"))
	if ready, _ := s.Ready(); ready {
		t.Error("expected not ready after a failed reconcile with no prior success")
	}
}
