# Changelog

All notable changes to this project are documented here.
Format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).
Versioning follows [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.1.4] - 2026-05-15

### Changed
- README completely rewritten: Docker usage with `network_mode: host`, full environment variable reference, label format examples, etcd configuration, and dev environment setup.
- Fixed etcd endpoint configuration example in documentation (array format, not single string).

## [0.1.3] - 2026-05-15

### Changed
- Go upgraded from 1.25 to 1.26.3.
- Removed deprecated `install` option from `setup-buildx-action` in CI.

---

> **Note:** Versions v0.2.0 through v0.6.0 below are the release history from the original
> `the original repository` repository, preserved here after migration to the `auto-dns` org.

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
