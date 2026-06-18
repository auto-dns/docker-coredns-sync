# Roadmap

Bugs, features, and tech debt for this project. Intended to be migrated into
**GitHub Issues** (one issue per item below), prioritized on a **Projects**
board and grouped into a release **Milestone** (proposed: `v0.7.0`).

Two items previously tracked here were confirmed fixed and removed:

- A records with no value defaulting to the host IP now works for aliased /
  identifier labels (e.g. `coredns.a.proxy.name`, `coredns.a.1.name`), not just
  the simple `coredns.a.name` form.
- Automatic correction of a record whose value drifted from desired state (the
  "changed `HOST_IP`" scenario): reconciliation already self-heals this for
  records owned by the local host, because `RecordIntent.Key()` includes the
  value and stale host-owned records are removed during reconciliation.

---

## P1 — Reliability & correctness

### Auto-reconnect (with backoff) for the Docker event stream  `bug`
When the Docker event stream closes (daemon restart, socket drop), the daemon
silently stops reacting to events while appearing healthy. In
`internal/event/docker_generator.go:74-78` the goroutine returns and closes its
output channel; in `internal/core/engine.go:65-69` the event-processing
goroutine then exits, **but the reconciliation loop keeps running on frozen
`MemoryState`**. The error-channel case only logs (`docker_generator.go:70-73`).

- Wrap subscribe/consume in a reconnect loop with exponential backoff + jitter.
- On channel close (distinct from `ctx` cancel), re-list containers and
  re-subscribe `Since` the last-seen timestamp, re-emitting initial-detection
  events so state re-syncs.
- Expose connection state for the readiness probe.
- Sub-task: make the event buffer size configurable (`docker_generator.go:27`
  has a literal `// TODO: config this`).
- Done when: restarting the Docker daemon recovers without a process restart;
  backoff is bounded/logged; mock-based test covers the reconnect path.

### Health & readiness endpoints  `enhancement`
Nothing external can tell whether the daemon is wedged (see reconnect bug above).
- Small `net/http` server exposing `/healthz` (liveness) and `/readyz`
  (readiness: Docker subscription active AND last etcd reconcile succeeded
  within N intervals).
- Engine updates a shared status struct; `/readyz` reads it.
- Config: `http.listen_addr` (default `:8080`) + enable flag.
- Shares the HTTP server with the metrics endpoint.

### Dry-run mode  `enhancement`
- `--dry-run` flag: in the reconcile loop (`engine.go`), log the
  `toAdd`/`toRemove` plan and skip the `reg.Register`/`reg.Remove` calls.
- Low effort, high value given how much logic lives in reconciliation.

## P2 — Security & observability

### etcd authentication & TLS  `enhancement`
- Extend `EtcdConfig` (`internal/config/config.go:30-36`) with `username`,
  `password`, and a `tls` block (`ca_file`, `cert_file`, `key_file`,
  `insecure_skip_verify`).
- Build `clientv3.Config` with credentials and a `*tls.Config`.
- Validate in `config.validate()` (cert+key must come together); the config
  already requires `http://`/`https://` endpoints but ignores TLS for `https://`
  today — close that gap.

### Metrics endpoint  `enhancement`
- Prometheus `/metrics` via `prometheus/client_golang` (shares HTTP server with
  health endpoints).
- Instrument: reconcile loop duration + last-success timestamp, records
  added/removed/skipped, etcd lock failures, etcd op errors, Docker stream
  disconnects.
- Gate behind `metrics.enabled` / `metrics.listen_addr`.

## P3 — Features & robustness

### GC orphaned records from dead/removed hosts  `enhancement` (needs design)
Same-host GC already works (host-owned records with no running container are
removed during reconciliation). The gap: a record owned by a hostname that no
longer exists is never cleaned up, because a host only touches its own records
(`internal/core/reconciliation.go:185-198`).
- Option A: each host writes a TTL'd liveness key; records owned by a host with
  no live heartbeat become GC-eligible by any host.
- Option B: an opt-in authoritative/reaper mode.
- Touches the multi-host ownership model — design discussion before coding.

### Per-record TTL control  `enhancement`
- The SkyDNS JSON value supports a `ttl` field; the writer doesn't set it today.
- Add `ttl` to the etcd value + write path (`internal/registry/etcd_registry.go`).
- Config default (`etcd.default_ttl`) + label override `coredns.a.ttl` /
  `coredns.a.<alias>.ttl`, parsed in `labels.go` alongside `force`.
- Decide whether a TTL-only change should count as record drift (likely yes).

### Live config reload  `enhancement`
- Viper already pulls in `fsnotify`; use `WatchConfig()` + `OnConfigChange`.
- Only hot-reload the safe subset (`log.level`, `poll_interval`, IP defaults);
  log-and-ignore unsafe changes (`hostname`, etcd endpoints — identity/ownership).
- Re-validate before applying.

### Harden startup config validation  `enhancement`
- `config.validate()` (`config.go:108-151`) is already thorough (hostname, IP
  format, etcd endpoints, lock params, log level).
- Add: warn clearly at startup when A records are in use but `host_ipv4` is
  unset (today such records are silently skipped at `record_builder.go:39-42`).

## P4 — Documentation

### Document label case-sensitivity + fix stale defaults  `documentation`
Confirmed behavior (worth documenting because it is inconsistent):
- Prefix (`coredns`): **case-sensitive** (`labels.go:69`).
- Record kind (`a`/`A`): **case-insensitive** (`ParseKind` → `ToUpper`,
  `internal/domain/record_factory.go:9`).
- Field (`name`/`value`/`force`): **case-sensitive**, must be lowercase
  (`labels.go:96`) — so `coredns.a.Name` silently fails.
- Boolean values (`enabled`/`force`): case-insensitive.

Also: the README documents `app.hostname` default `"your-hostname"`, but the
real default is `""` and an empty hostname is rejected by validation. Fix the
README.

## P5 — Low priority (may or may not be implemented)

### TXT record support  `enhancement`
Low value in a homelab. Main realistic use is ACME DNS-01 challenges, which are
dynamic per-issuance and not container-label-driven. Defer unless a concrete
need appears.

### SRV record support (incl. weight/priority passthrough)  `enhancement`
Niche but real homelab uses: Minecraft (`_minecraft._tcp`), Matrix
(`_matrix._tcp`), CalDAV/CardDAV autodiscovery. The SkyDNS value's `priority`
(failover ordering) and `weight` (load distribution among equal priorities)
fields are only meaningful for SRV, so bundle that passthrough here.
