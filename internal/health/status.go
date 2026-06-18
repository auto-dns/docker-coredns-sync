package health

import (
	"sync"
	"time"
)

// Status is a concurrency-safe view of the daemon's runtime health. It is fed
// by the sync engine (reconciliation outcomes, via RecordReconcile) and the
// Docker event generator (connection state, via SetDockerConnected), and read
// by the readiness handler.
type Status struct {
	mu                   sync.RWMutex
	dockerConnected      bool
	lastReconcileSuccess time.Time
	lastReconcileErr     error
	readyThreshold       time.Duration
	dryRun               bool

	// now is overridable in tests.
	now func() time.Time
}

// NewStatus creates a Status. readyThreshold is the maximum age of the last
// successful reconciliation for the daemon to be considered ready.
func NewStatus(readyThreshold time.Duration) *Status {
	return &Status{
		readyThreshold: readyThreshold,
		now:            time.Now,
	}
}

// SetDockerConnected records whether the Docker event stream is connected.
func (s *Status) SetDockerConnected(connected bool) {
	s.mu.Lock()
	s.dockerConnected = connected
	s.mu.Unlock()
}

// SetDryRun marks the daemon as running in dry-run mode, in which it applies no
// records and therefore never reports ready.
func (s *Status) SetDryRun(dryRun bool) {
	s.mu.Lock()
	s.dryRun = dryRun
	s.mu.Unlock()
}

// RecordReconcile records the outcome of a reconciliation pass. A nil error
// marks the pass as successful and refreshes the readiness timestamp.
func (s *Status) RecordReconcile(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastReconcileErr = err
	if err == nil {
		s.lastReconcileSuccess = s.now()
	}
}

// Ready reports whether the daemon is ready to serve, with a human-readable
// reason when it is not. Readiness requires the Docker stream to be connected
// and a reconciliation to have succeeded within the readiness threshold.
func (s *Status) Ready() (bool, string) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.dryRun {
		return false, "dry-run mode: records are not applied"
	}
	if !s.dockerConnected {
		return false, "docker event stream not connected"
	}
	if s.lastReconcileSuccess.IsZero() {
		return false, "no successful reconciliation yet"
	}
	if s.now().Sub(s.lastReconcileSuccess) > s.readyThreshold {
		return false, "last successful reconciliation is older than the readiness threshold"
	}
	return true, "ok"
}
