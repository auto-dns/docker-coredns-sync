# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```bash
go test ./...                    # Run all tests
go test -race ./...              # Race detection
go test -coverprofile=coverage.out ./... && go tool cover -func=coverage.out
make lint                        # golangci-lint (requires golangci-lint installed)
make format                      # go fmt + goimports
```

## Architecture

```
Docker Events → SyncEngine → in-memory state → reconciliation loop → etcd (CoreDNS format)
```

- `internal/event/` — Docker event subscription and wiring
- `internal/core/` — `SyncEngine`, reconciliation, label parsing, validation
- `internal/domain/` — `ContainerEvent`, `DnsRecord`, `RecordIntent`, `RecordFactory`
- `internal/state/` — in-memory state tracking of which container owns which records
- `internal/registry/` — etcd client, key-path encoding (`/skydns/<reversed-domain>/<uuid>`)

The engine subscribes to Docker events, updates in-memory state, and on each poll interval takes an etcd distributed lock and reconciles desired vs actual records.

### Configuration

Docker label prefix defaults to `coredns`. Containers opt in with `coredns.enabled=true`. Records are defined via `coredns.a.name`, `coredns.a.value`, `coredns.cname.name`, etc. Aliased records use `coredns.a.<alias>.name` format.

---

## Development Workflow

**Never commit directly to `main`.** All changes go through a branch and PR.

### Branch naming

- `feat/<short-description>` — new features
- `fix/<short-description>` — bug fixes
- `chore/<short-description>` — maintenance, tooling, docs, dependency updates
- `version/<X.Y.Z>` — version bump + CHANGELOG update PRs

### Step-by-step process

```bash
# 1. Branch from main
git checkout main && git pull
git checkout -b feat/my-feature

# 2. Implement changes

# 3. Run local checks — ALL must pass before opening a PR
go build ./...          # compile check — catches type errors, duplicate declarations, etc.
go vet ./...            # static analysis
go test ./...           # unit tests

# 4. Push and open a PR
git push -u origin feat/my-feature
gh pr create --title "..." --body "..."

# 5. Antagonistic code review
#    Run /ultrareview in Claude Code to get an independent, critical review of the PR.
#    Address ALL feedback before merging. This is mandatory, not optional.

# 6. Merge the PR (squash merge preferred)
```

### Why local checks are mandatory

CI only runs on tag pushes, not branch pushes. A compile error will not surface until the Docker build on a tag — by which point the broken tag is already public. Always run `go build ./...` before creating a PR.

### Antagonistic code review

Before merging any PR, run `/ultrareview` (or `/ultrareview <PR#>`) in Claude Code. This spawns an independent review agent that critiques the PR adversarially — looking for bugs, race conditions, security issues, and API contract violations. Treat findings as blocking: address every concern or justify why it doesn't apply.

---

## Releasing

Releases are tag-driven. Pushing a `v*.*.*` tag triggers CI (`.github/workflows/docker.yaml`) to:
1. Build and push the Docker image to `ghcr.io/auto-dns/docker-coredns-sync`
2. Create a GitHub release automatically from the matching `CHANGELOG.md` section

Tags on `main` only. Stable releases use `vMAJOR.MINOR.PATCH`; pre-releases use `vMAJOR.MINOR.PATCH-suffix`.

### Release checklist

```bash
# 1. Update CHANGELOG.md on main (via PR):
#    - Change "## [Unreleased]" → "## [X.Y.Z] - YYYY-MM-DD"
#    - Add a new empty "## [Unreleased]" section at the top

# 2. After the CHANGELOG PR merges, tag main:
git checkout main && git pull
git tag -a vX.Y.Z -m "vX.Y.Z"
git push origin vX.Y.Z
```

CI then:
- Builds multi-platform image (amd64, arm64, arm/v7)
- Pushes `ghcr.io/auto-dns/docker-coredns-sync:X.Y.Z`
- For stable releases: also updates `:X.Y`, `:X`, and `:latest`
- Creates a GitHub release with the CHANGELOG section + Docker pull command

### CHANGELOG format

`CHANGELOG.md` follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) with sub-sections `Added`, `Changed`, `Fixed`, `Removed`, `Security`. The CI release step extracts the `## [X.Y.Z]` section by version number — the section heading must match `## [X.Y.Z]` exactly (no `v` prefix).
