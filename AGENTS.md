# AGENTS.md

This file provides guidance to Codex (Codex.ai/code) when working with code in this repository.

## Build & Run

```bash
# Build the binary
go build ./cmd/docker-coredns-sync/...

# Run locally (requires a config file or env vars)
go run ./cmd/docker-coredns-sync/...

# Build Docker image (production)
make build

# Build and run via Docker Compose (prod)
make up
make down
```

## Testing

The project has a comprehensive Go test suite (~98% statement coverage); nearly every source file has a matching `_test.go`. Docker and etcd are exercised through mocks, so no external services are required.

```bash
# Run all tests
make test            # or: go test ./...

# Verbose / race detector
make test-verbose
make test-race

# Coverage (func summary, or HTML report)
make test-coverage
make test-coverage-html

# Lint (requires golangci-lint) and format
make lint
make format          # go fmt + goimports
```

Manual integration test cases are additionally documented in `TESTS.md`.

## Development Environment

A devcontainer is provided in `.devcontainer/` with etcd and the app wired up via Docker Compose. After opening in a devcontainer, the `post-create.sh` script runs setup steps. The devcontainer uses Docker-outside-of-Docker so you can run containers from within it.

A local `config.yaml` (gitignored) in the project root is the easiest way to configure a dev run.

## Architecture

The application watches Docker container events and syncs DNS records into etcd using the [SkyDNS/CoreDNS etcd plugin format](https://coredns.io/plugins/etcd/).

### Data flow

1. **`cmd/docker-coredns-sync/root.go`** — Cobra CLI entrypoint. Loads config via Viper (flags → env vars → config file → defaults), sets up signal handling, creates and runs the app.
2. **`internal/app/app.go`** — Constructs the `DockerGenerator` and `EtcdRegistry`, then starts the `SyncEngine`.
3. **`internal/core/engine.go` (`SyncEngine`)** — Central coordinator:
   - Subscribes to Docker events via `DockerGenerator`
   - Pre-populates in-memory state from currently running containers on startup
   - Processes `start`/`stop` events to update `MemoryState`
   - Runs a reconciliation loop every `poll_interval` seconds under a distributed etcd lock
4. **`internal/core/record_builder.go`** — Parses Docker labels from a container event into `RecordIntent` objects. Handles both simple (`coredns.A.name`) and aliased (`coredns.A.proxy.name`) label formats.
5. **`internal/state/memory_state.go` (`MemoryState`)** — Thread-safe in-memory map of container ID → `containerState`. Only "running" containers contribute desired records.
6. **`internal/core/reconciliation.go`** — Two-stage reconciliation:
   - `FilterRecordIntents`: resolves conflicts *within* the desired set (multiple containers wanting the same DNS name). Priority: `force` label beats non-force; older container wins when force flags are equal. A/CNAME conflicts are also resolved here.
   - `ReconcileAndValidate`: compares desired vs. actual etcd state. Produces `toAdd`/`toRemove` slices. Only removes records owned by this hostname (`cfg.Hostname`). Handles cross-host eviction via `force` and container age.
7. **`internal/registry/etcd_registry.go` (`EtcdRegistry`)** — Implements the `Registry` interface against etcd. Keys follow SkyDNS format: `{path_prefix}/{reversed-domain}/x{index}` (e.g., `/skydns/com/example/app/x1`). Values are JSON with `host`, `record_type`, `owner_hostname`, `owner_container_id`, `owner_container_name`, `created`, and `force`.

### Key types

- **`RecordIntent`** (`internal/domain/`) — Wraps a DNS record with ownership metadata (container ID/name, hostname, created timestamp, force flag). This is the unit of work throughout the system.
- **`Record`** (`internal/domain/`) — A value struct carrying a `RecordKind` (`A`, `AAAA`, or `CNAME`), name, and value. Constructed via `NewA`/`NewAAAA`/`NewCNAME` (or `NewFromKind`), which validate the hostname and address.
- **`nestedRecordMap`** (`internal/core/nested_maps.go`) — Three-level map `name → kind → value → RecordIntent` used during reconciliation to group and look up records efficiently.

### Conflict resolution rules

When two containers want the same DNS name:
- `force=true` beats `force=false`
- When both have the same force value, the **older** container (earlier `Created` timestamp) wins
- A record and CNAME for the same name cannot coexist; the winner is determined by the same rules

### Config

Config is handled by Viper in `internal/config/config.go`. The etcd connection takes `endpoints` as a `[]string` (e.g., `["http://etcd:2379"]`). `app.hostname` must be unique per node — it's used to scope ownership of etcd records.

## Versioning, branches & releases

See `CONTRIBUTING.md` for the full contributor guide. Key conventions:

- **Versioning:** [SemVer](https://semver.org/); active line is **0.7.x**. Record changes in `CHANGELOG.md` (Keep a Changelog) under `## [Unreleased]` in the same PR.
- **Branches:** `main` is the default/stable branch. Each release is integrated on a release branch named `vMAJOR.MINOR.PATCH` (e.g. `v0.7.0`); open PRs against the **active release branch**, not `main`. The release branch later merges into `main` and is tagged.
- **Tags & releases:** pushing a tag `vMAJOR.MINOR.PATCH` (pre-release: `vMAJOR.MINOR.PATCH-SUFFIX`) triggers `.github/workflows/docker.yaml`, which builds/pushes the GHCR image and cuts a GitHub Release from the matching `## [MAJOR.MINOR.PATCH]` CHANGELOG section. Rename `## [Unreleased]` to the versioned heading before tagging.

## Pull request conventions

- Target the active release branch (`vMAJOR.MINOR.PATCH`), not `main`.
- Link issues with closing keywords in the PR body — `Closes #N` / `Fixes #N` / `Resolves #N`. Merging to `main` auto-closes them; merging to a release branch (`v*`) labels them `awaiting-release` via `.github/workflows/awaiting-release.yaml`.
- **Issue state** encodes pipeline position: open without `awaiting-release` = open for development; open + `awaiting-release` = merged to a release branch, not yet released; closed = released.
- Keep changes covered by tests (`go test -race ./...`), run `make lint`/`make format`, and update `CHANGELOG.md`.
