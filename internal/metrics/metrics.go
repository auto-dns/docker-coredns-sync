// Package metrics provides the Prometheus instrumentation for the daemon. The
// collectors are registered on a private registry (not the global default) so
// the set is self-contained and safe to construct repeatedly in tests.
package metrics

import (
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Metrics holds the daemon's Prometheus collectors and the registry they are
// registered on. It is fed by the sync engine (reconcile outcomes), the etcd
// registry (operation/lock errors), and the Docker generator (disconnects).
type Metrics struct {
	registry *prometheus.Registry

	reconcileDuration    prometheus.Histogram
	reconcileTotal       *prometheus.CounterVec
	lastReconcileSuccess prometheus.Gauge
	recordsAdded         prometheus.Counter
	recordsRemoved       prometheus.Counter
	recordsSkipped       prometheus.Gauge
	etcdErrors           prometheus.Counter
	etcdLockFailures     prometheus.Counter
	dockerDisconnects    prometheus.Counter

	// dryRun is set once at startup. In dry-run the daemon applies nothing, so a
	// pass is not counted as a success and the last-success gauge is not
	// refreshed (mirroring readiness, which also reports not-ready).
	dryRun bool
}

// New constructs a Metrics set with all collectors registered on a fresh
// registry.
func New() *Metrics {
	reg := prometheus.NewRegistry()
	m := &Metrics{
		registry: reg,
		reconcileDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "dcs_reconcile_duration_seconds",
			Help:    "Duration of a reconciliation pass in seconds.",
			Buckets: prometheus.DefBuckets,
		}),
		reconcileTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "dcs_reconcile_total",
			Help: "Total number of reconciliation passes by result.",
		}, []string{"result"}),
		lastReconcileSuccess: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "dcs_reconcile_last_success_timestamp_seconds",
			Help: "Unix timestamp of the last successful reconciliation.",
		}),
		recordsAdded: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "dcs_records_added_total",
			Help: "Total number of DNS records added to etcd.",
		}),
		recordsRemoved: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "dcs_records_removed_total",
			Help: "Total number of DNS records removed from etcd.",
		}),
		recordsSkipped: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "dcs_records_skipped",
			Help: "Number of desired records dropped during conflict filtering on the most recent reconciliation. This is a steady-state count, not a cumulative total.",
		}),
		etcdErrors: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "dcs_etcd_errors_total",
			Help: "Total number of etcd operation errors.",
		}),
		etcdLockFailures: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "dcs_etcd_lock_failures_total",
			Help: "Total number of etcd distributed-lock acquisition failures.",
		}),
		dockerDisconnects: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "dcs_docker_disconnects_total",
			Help: "Total number of Docker event-stream disconnects.",
		}),
	}
	reg.MustRegister(
		m.reconcileDuration,
		m.reconcileTotal,
		m.lastReconcileSuccess,
		m.recordsAdded,
		m.recordsRemoved,
		m.recordsSkipped,
		m.etcdErrors,
		m.etcdLockFailures,
		m.dockerDisconnects,
	)
	return m
}

// SetDryRun marks the daemon as running in dry-run mode. Must be called before
// the reconciliation loop starts. See the dryRun field for the effect.
func (m *Metrics) SetDryRun(dryRun bool) { m.dryRun = dryRun }

// Handler returns the HTTP handler that exposes this metrics set in the
// Prometheus text exposition format.
func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{})
}

// ObserveReconcile records the outcome of a reconciliation pass: its duration,
// the number of records added/removed (cumulative counters) and currently
// skipped during conflict filtering (a gauge), and whether the pass failed.
// A successful non-dry-run pass refreshes the last-success gauge.
func (m *Metrics) ObserveReconcile(duration time.Duration, added, removed, skipped int, err error) {
	m.reconcileDuration.Observe(duration.Seconds())
	m.recordsAdded.Add(float64(added))
	m.recordsRemoved.Add(float64(removed))
	m.recordsSkipped.Set(float64(skipped))
	switch {
	case err != nil:
		m.reconcileTotal.WithLabelValues("error").Inc()
	case m.dryRun:
		// A dry-run pass applies nothing, so it is neither a success nor an
		// error and must not refresh the last-success gauge.
		m.reconcileTotal.WithLabelValues("dry_run").Inc()
	default:
		m.reconcileTotal.WithLabelValues("success").Inc()
		m.lastReconcileSuccess.Set(float64(time.Now().Unix()))
	}
}

// IncEtcdError increments the etcd operation-error counter.
func (m *Metrics) IncEtcdError() { m.etcdErrors.Inc() }

// IncLockFailure increments the etcd lock-acquisition-failure counter.
func (m *Metrics) IncLockFailure() { m.etcdLockFailures.Inc() }

// IncDockerDisconnect increments the Docker event-stream disconnect counter.
func (m *Metrics) IncDockerDisconnect() { m.dockerDisconnects.Inc() }
