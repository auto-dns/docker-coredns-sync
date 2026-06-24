# Changelog

All notable changes to this project are documented here.
Format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).
Versioning follows [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

> **Versioning note.** This project migrated from its original repository to the
> `auto-dns` organization. Its canonical version line continues from the highest
> pre-migration release, **v0.6.0** — **v0.6.1** is the first release cut on the
> post-migration `main` history, and development continues on the **0.6.x** line.
>
> Two earlier numbering artifacts are kept below for historical accuracy but are
> **not** the active line:
> - **v0.1.0 / v0.1.3 / v0.1.4** — a short-lived restart of versioning just after
>   the migration, since abandoned in favor of continuing from v0.6.0.
> - **v0.2.0 – v0.6.0** — release history inherited from the original repository.
>   These tags point at pre-migration commits that are **not reachable from the
>   current `main`**.

## [0.7.0] - 2026-06-24

### Added
- `CONTRIBUTING.md` documenting versioning, branch, tag/release, issue, and
  pull-request conventions; a pull request template; and a workflow that labels
  issues `awaiting-release` when their PR merges into a release branch.
- etcd authentication and TLS (`etcd.username`, `etcd.password`, and an
  `etcd.tls` block with `ca_file`, `cert_file`, `key_file`,
  `insecure_skip_verify`), enabling connections to auth- and/or TLS-protected
  etcd, including mutual TLS. Misconfiguration fails fast at startup: a username
  without a password (or a password without a username), a cert without its key,
  an unreadable CA, or a TLS block configured while no endpoint uses `https://`
  (so the settings would be silently ignored). `etcd.password` intentionally has
  no CLI flag — set it via the env var or config file so it is never exposed in
  the process list. (#11)
- Prometheus metrics endpoint (`metrics.enabled`): exposes `/metrics` on the
  shared HTTP server with reconcile duration/last-success/result, records
  added/removed/skipped, etcd op/lock-failure counters, and Docker-disconnect
  counters. The HTTP server now starts when either `http.enabled` or
  `metrics.enabled` is set. (#12)
- Dry-run mode (`app.dry_run` / `--app.dry-run`): the reconciliation loop logs
  the planned add/remove set and makes no changes to etcd. (#10)
- Health and readiness HTTP endpoints (`http.enabled`, `http.listen_addr`):
  `/healthz` for liveness and `/readyz` for readiness (Docker stream connected
  and a recent successful reconciliation). (#9)
- Configurable Docker event buffer and reconnect backoff
  (`docker.event_buffer_size`, `docker.reconnect_initial_backoff`,
  `docker.reconnect_max_backoff`). (#8)
- **Per-record TTL control.** Records can now carry a TTL: set a global default
  with `app.record_ttl` (`--app.record-ttl`, seconds) or override per record with a
  `coredns.<kind>[.<alias>].ttl` label. `0` leaves the TTL unset so CoreDNS
  applies its own default. A TTL change is treated as record drift, so it
  self-heals on the next reconcile. (#14)
- **Cross-host garbage collection of orphaned records.** Every host publishes a
  lease-backed heartbeat key outside `etcd.path_prefix` and keeps it alive;
  heartbeating is always on. Reconciliation removes records whose owner has no
  live heartbeat, so a permanently removed node is cleaned up automatically once
  its lease expires — no manual step. The lease TTL (`app.heartbeat_ttl` /
  `--app.heartbeat-ttl`, default `30s`, must be > 0) is the grace period, so transient outages don't trigger
  premature deletion. A host runs GC only while it is itself actively
  heartbeating (so a failed heartbeat registration disables its GC rather than
  letting it act on liveness it can't vouch for), and the liveness lookup uses a
  linearizable etcd read because it authorizes deletions. If the lease is later
  lost (e.g. an etcd outage longer than the TTL), the host disables its own GC
  and re-establishes the lease with backoff, so liveness self-heals without a
  restart. The reserved heartbeat key prefix is validated not to overlap
  `etcd.path_prefix`. Dry-run does not heartbeat (it makes no etcd writes). (#13)
- **Prominent startup warning** when `app.host_ipv4`/`app.host_ipv6` is unset,
  making it obvious that value-less A/AAAA records will be skipped. (#16)

### Changed
- The Docker event stream now reconnects automatically with bounded
  exponential backoff when it drops, instead of silently going dead while the
  reconciliation loop kept running on stale in-memory state. On each
  (re)connection the full running-container set is re-synced and state is pruned
  of containers that stopped while disconnected (e.g. across a daemon restart),
  the backoff resets after a healthy connection, and a closed error channel
  triggers a reconnect rather than a silent stall. (#8)
- The health server now fails fast at startup if its listen address cannot be
  bound, instead of logging and continuing without endpoints. Readiness reports
  not-ready in dry-run mode and when record writes to etcd fail (previously a
  pass with failing writes was reported as successful). (#8, #9, #10)
- Resync pruning of containers missing after a reconnect is debounced (a
  container must be absent for two consecutive resyncs) so a container that is
  only transiently missing from a single snapshot is not removed. (#8)

### Documentation
- Documented Docker label case-sensitivity rules in the README: the prefix,
  field names (`name`/`value`/`force`/`ttl`), and aliases are case-sensitive
  (a wrong-case segment is silently ignored), while record kinds and boolean
  values are case-insensitive. Fixed stale config-reference defaults: `app.hostname`
  is `""` and required (not `"your-hostname"`), and the non-existent `app.host_ip`
  / `127.0.0.1` entry is replaced with the real `app.host_ipv4` / `app.host_ipv6`
  keys (both default `""`). Also corrected the AAAA label example, which had been
  copy-pasted from the A record. (#17)

### Fixed
- A clean, signal-driven shutdown now exits with status 0 instead of 1
  (`context.Canceled` is treated as a normal stop), so process supervisors no
  longer see a graceful stop as a crash.
- The etcd client is now closed exactly once on shutdown (previously both the
  engine and the app closed it).
- The in-memory container map no longer grows without bound: stopped and pruned
  containers are deleted rather than retained in a removed state.
- Readiness now reports not-ready immediately after a failed reconciliation pass
  (not only once the last success ages out), and the auxiliary HTTP server sets
  write and idle timeouts in addition to the read-header timeout.

### Upgrade guide (0.6.x → 0.7.0)

This release introduces always-on, lease-backed heartbeats and automatic
cross-host garbage collection of orphaned records. No configuration keys were
renamed or removed and every new setting has a default, so existing config files
and environment variables keep working unchanged — but the new GC behavior means
a few steps are required:

1. **Upgrade all hosts together.** A host running an older version does not
   publish a heartbeat, so upgraded hosts treat it as dead and garbage-collect
   its records. Roll the whole fleet in a single window rather than host-by-host;
   do not run 0.6.x and 0.7.0 against the same etcd for an extended period.
2. **Make sure `etcd.path_prefix` does not overlap the heartbeat keyspace.**
   Heartbeats are written under the reserved prefix
   `/docker-coredns-sync/heartbeat`, deliberately outside `etcd.path_prefix`.
   Startup now **fails fast** if the two overlap. The default (`/skydns`) is
   unaffected — only a custom prefix that collides needs changing. If you run
   etcd with authentication/RBAC, grant the daemon read/write access to this
   reserved keyspace in addition to `etcd.path_prefix`.
3. **Expect orphaned records to be reclaimed automatically.** Once every host is
   upgraded, records owned by a host that is gone are removed after its heartbeat
   lease expires. To retire a host, simply stop its daemon — no manual etcd
   cleanup is needed. Tune the grace period with `app.heartbeat_ttl`
   (`--app.heartbeat-ttl`, default `30s`, must be > 0); transient outages shorter
   than this do not trigger deletion.

Downgrading is safe: a 0.6.x binary ignores the heartbeat keys, which expire on
their own via their lease.

## [0.6.1] - 2026-06-17

Maintenance release. No functional or configuration changes — the application's
runtime behavior is unchanged from the previous release.

### Changed
- Typed the container event and status enums; replaced `"running"`/`"removed"`
  magic strings with `domain.ContainerStatus` constants.
- Use the standard library `io.Closer` in place of a local interface; lowercased
  internal error strings to follow Go conventions.
- Formatted the entire codebase with `gofmt`.

### Removed
- Unused internal utility helpers and dead record-map methods.

## [0.1.4] - 2026-05-15

### Changed
- README completely rewritten: Docker usage with `network_mode: host`, full environment variable reference, label format examples, etcd configuration, and dev environment setup.
- Fixed etcd endpoint configuration example in documentation (array format, not single string).

## [0.1.3] - 2026-05-15

### Changed
- Go upgraded from 1.25 to 1.26.3.
- Removed deprecated `install` option from `setup-buildx-action` in CI.

---

> The releases below (**v0.2.0 – v0.6.0**) predate the migration to the `auto-dns`
> org and are preserved for historical reference. See the versioning note at the
> top of this file.

---

## [0.6.0] - 2026-05-10

### Added
- Unit test suite covering sync engine, label parsing, record factory, and etcd registry.

### Changed
- Go upgraded from 1.24 to 1.25; vulnerable dependencies patched.
- `--etcd-endpoint` flag renamed to `--etcd-endpoints` for consistency with the config key name.
- etcd client now accepts a list of endpoints (was previously a single string).

## [0.5.3] - 2025-09-16

### Fixed
- Improved etcd lease handling: leases are now reliably renewed and released even under high-throughput container churn.

## [0.5.2] - 2025-09-11

### Fixed
- Hotfix for lease acquisition race condition that could leave stale records in etcd after a container stop event.

## [0.5.1] - 2025-09-11

### Fixed
- Minor corrections to IPv6 record field mapping after the v0.5.0 AAAA support landed.

## [0.5.0] - 2025-09-11

### Added
- IPv6 support: containers with a `coredns.aaaa.*` label now register AAAA records in etcd alongside A records.

## [0.4.0] - 2025-07-26

### Changed
- etcd endpoint configuration now accepts an array (`etcd.endpoints: [...]`) instead of a single string, allowing the sync daemon to connect to all nodes in a 3-node etcd cluster.

## [0.3.3] - 2025-05-30

### Fixed
- Aliased A record default value was not being populated correctly, causing missing records for containers using the `coredns.a.<alias>.name` label pattern.

## [0.3.2] - 2025-04-18

### Fixed
- Minor dependency and go.sum corrections.

## [0.3.1] - 2025-04-17

### Fixed
- Reconciliation loop stability improvements.

## [0.3.0] - 2025-04-17

### Changed
- Removed DNS record interface abstraction; simplified domain model.
- Dev environment now seeded via `config.yaml` instead of environment variables.

## [0.2.0] - 2025-04-17

### Changed
- Full rewrite in Go (replaced prior implementation).
- SkyDNS/CoreDNS etcd format for all record types.
- `coredns.*` Docker label schema: `coredns.enabled`, `coredns.a.name`, `coredns.a.value`, `coredns.cname.name`, `coredns.cname.value`, aliased form `coredns.a.<alias>.*`.
- Distributed etcd lock ensures only one sync daemon reconciles at a time across multi-host deployments.
- In-memory state tracks which container owns which records; reconciliation removes orphaned records on container stop/die.

## [0.1.0] - 2025-04-03

### Added
- Initial release.
