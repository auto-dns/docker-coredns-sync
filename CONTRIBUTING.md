# Contributing

Thanks for contributing to `docker-coredns-sync`. This guide documents the
project's conventions for versioning, branches, tags, issues, and pull requests.

> Licensing note: this project uses a [custom MIT-NC license](./LICENSE)
> (non-commercial). By contributing you agree your contributions are licensed
> under the same terms.

## Development quickstart

```bash
go build ./cmd/docker-coredns-sync/...   # build
make test                                 # or: go test ./...
make test-race                            # race detector
make lint                                 # golangci-lint
make format                               # go fmt + goimports
```

Docker and etcd are mocked in tests, so no external services are required. See
`CLAUDE.md` for an architecture overview and `TESTS.md` for manual integration
cases.

## Versioning

- The project follows [Semantic Versioning](https://semver.org/) (`MAJOR.MINOR.PATCH`).
- The active version line is **0.7.x**, continuing from the pre-migration
  **0.6.x** history. See the versioning note at the top of `CHANGELOG.md` for
  the full history.
- All notable changes are recorded in `CHANGELOG.md` following
  [Keep a Changelog](https://keepachangelog.com/). Add your entry under the
  `## [Unreleased]` heading in the same PR as your change.

## Branches

- **`main`** is the default, stable branch.
- Each release is integrated on a **release branch** named `vMAJOR.MINOR.PATCH`
  (e.g. `v0.7.0`). Open feature and fix PRs against the **active release
  branch**, not `main`.
- When a release branch is ready, it is merged into `main` and tagged (see
  below). Issues referenced by merged PRs auto-close at that point.

## Tags & releases

- Releases are cut by pushing a git tag:
  - Stable: `vMAJOR.MINOR.PATCH` (e.g. `v0.7.0`).
  - Pre-release: `vMAJOR.MINOR.PATCH-SUFFIX` (e.g. `v0.7.0-rc.1`).
- Pushing a matching tag triggers `.github/workflows/docker.yaml`, which:
  - builds and pushes the multi-arch image to GHCR, and
  - creates a GitHub Release with notes extracted from the matching
    `## [MAJOR.MINOR.PATCH]` section of `CHANGELOG.md`.
- Stable (non-pre-release) tags also move the `MAJOR`, `MAJOR.MINOR`, and
  `latest` image tags.
- Before tagging, rename the `## [Unreleased]` CHANGELOG section to
  `## [MAJOR.MINOR.PATCH] - YYYY-MM-DD`.

## Issues & labels

Issue state encodes where work is in the release pipeline:

| State | Meaning |
|-------|---------|
| Open, no `awaiting-release` label | Open for development |
| Open + `awaiting-release` | Implemented and merged to a release branch; not yet released |
| Closed | Released (auto-closed when the release branch merges to `main`) |

Common labels: `bug`, `enhancement`, `documentation`, `awaiting-release`.
Group issues by their target release using a **milestone** (e.g. `v0.7.0`).

## Pull requests

- **Target the active release branch** (`vMAJOR.MINOR.PATCH`), not `main`.
- **Link issues with closing keywords** in the PR body: `Closes #N`,
  `Fixes #N`, or `Resolves #N`. This drives two automations:
  - merging into `main` auto-closes the referenced issues;
  - merging into a release branch (`v*`) labels them `awaiting-release` via
    `.github/workflows/awaiting-release.yaml`.
- Keep changes covered by tests (`go test -race ./...`) and run `make lint` /
  `make format` before opening the PR.
- Update `CHANGELOG.md` under `## [Unreleased]`.
