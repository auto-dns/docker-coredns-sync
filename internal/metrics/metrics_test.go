package metrics

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestObserveReconcile_Success(t *testing.T) {
	m := New()
	m.ObserveReconcile(50*time.Millisecond, 2, 1, 3, nil)

	if got := testutil.ToFloat64(m.recordsAdded); got != 2 {
		t.Errorf("records added = %v, want 2", got)
	}
	if got := testutil.ToFloat64(m.recordsRemoved); got != 1 {
		t.Errorf("records removed = %v, want 1", got)
	}
	if got := testutil.ToFloat64(m.recordsSkipped); got != 3 {
		t.Errorf("records skipped gauge = %v, want 3", got)
	}
	if got := testutil.ToFloat64(m.reconcileTotal.WithLabelValues("success")); got != 1 {
		t.Errorf("reconcile success total = %v, want 1", got)
	}
	if got := testutil.ToFloat64(m.lastReconcileSuccess); got <= 0 {
		t.Errorf("last success timestamp = %v, want > 0", got)
	}
}

func TestObserveReconcile_SkippedIsGaugeNotCumulative(t *testing.T) {
	m := New()
	m.ObserveReconcile(time.Millisecond, 0, 0, 5, nil)
	m.ObserveReconcile(time.Millisecond, 0, 0, 2, nil)
	// A gauge reflects the most recent pass, not the sum (which would be 7).
	if got := testutil.ToFloat64(m.recordsSkipped); got != 2 {
		t.Errorf("skipped gauge = %v, want 2 (latest pass, not cumulative)", got)
	}
}

func TestObserveReconcile_DryRunNotSuccess(t *testing.T) {
	m := New()
	m.SetDryRun(true)
	m.ObserveReconcile(time.Millisecond, 0, 0, 0, nil)

	if got := testutil.ToFloat64(m.reconcileTotal.WithLabelValues("dry_run")); got != 1 {
		t.Errorf("dry_run total = %v, want 1", got)
	}
	if got := testutil.ToFloat64(m.reconcileTotal.WithLabelValues("success")); got != 0 {
		t.Errorf("success total = %v, want 0 in dry-run", got)
	}
	if got := testutil.ToFloat64(m.lastReconcileSuccess); got != 0 {
		t.Errorf("last success timestamp = %v, want 0 (never refreshed in dry-run)", got)
	}
}

func TestObserveReconcile_Error(t *testing.T) {
	m := New()
	m.ObserveReconcile(time.Millisecond, 0, 0, 0, errors.New("boom"))

	if got := testutil.ToFloat64(m.reconcileTotal.WithLabelValues("error")); got != 1 {
		t.Errorf("reconcile error total = %v, want 1", got)
	}
	if got := testutil.ToFloat64(m.lastReconcileSuccess); got != 0 {
		t.Errorf("last success timestamp should stay 0 on error, got %v", got)
	}
}

func TestIncCounters(t *testing.T) {
	m := New()
	m.IncEtcdError()
	m.IncEtcdError()
	m.IncLockFailure()
	m.IncDockerDisconnect()

	if got := testutil.ToFloat64(m.etcdErrors); got != 2 {
		t.Errorf("etcd errors = %v, want 2", got)
	}
	if got := testutil.ToFloat64(m.etcdLockFailures); got != 1 {
		t.Errorf("etcd lock failures = %v, want 1", got)
	}
	if got := testutil.ToFloat64(m.dockerDisconnects); got != 1 {
		t.Errorf("docker disconnects = %v, want 1", got)
	}
}

func TestHandler_ExposesMetrics(t *testing.T) {
	m := New()
	m.IncEtcdError()
	m.ObserveReconcile(time.Millisecond, 1, 0, 0, nil)

	srv := httptest.NewServer(m.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 from /metrics, got %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	out := string(body)

	for _, want := range []string{
		"dcs_reconcile_duration_seconds",
		"dcs_records_added_total",
		"dcs_etcd_errors_total",
		"dcs_etcd_lock_failures_total",
		"dcs_docker_disconnects_total",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("expected metrics output to contain %q", want)
		}
	}
}
