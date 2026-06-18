# docker-coredns-sync

`docker-coredns-sync` listens for Docker container events and automatically registers/deregisters A and CNAME records in etcd using the [SkyDNS/CoreDNS etcd plugin format](https://coredns.io/plugins/etcd/). This enables dynamic DNS resolution for containers via CoreDNS.

---

## Features

- Supports **A** and **CNAME** records
- Multiple domain support per container
- Prevents CNAME cycles
- Automatically removes stale records
- **Per-record TTL** control via config default or label override
- **Multi-host aware**: each host publishes a liveness heartbeat and garbage-collects records left behind by hosts that are permanently gone
- Graceful shutdown support
- Flexible configuration via **flags**, **env vars**, and **config file**
- Supports both **YAML** and **JSON** config formats

---

## Docker Label Configuration

### Required

- `coredns.enabled=true` — Enables DNS registration for this container.

### A Record

- `coredns.a.name=foo.example.com`
- `coredns.a.value=192.168.1.123` *(optional, defaults to `host_ipv4`)*

### AAAA Record
- `coredns.a.name=foo.example.com`
- `coredns.a.value=fd20:0:1::123` *(optional, defaults to `host_ipv6`)*

### CNAME Record

- `coredns.cname.name=bar.example.com`
- `coredns.cname.value=foo.example.com`

### Aliased Records

Supports multiple A/CNAME records via aliases:

```yaml
coredns.a.proxy.name=proxy.example.com
coredns.a.proxy.value=192.168.200.2

coredns.cname.app.name=app.example.com
coredns.cname.app.value=target.example.com
```

### Optional

- `coredns.force=true` — Force registration for all records in the container
- `coredns.a.force=true` — Force a specific A record
- `coredns.a.ttl=300` — Per-record TTL in seconds. Overrides the `app.record_ttl`
  default. For aliased records use `coredns.a.<alias>.ttl` (e.g.
  `coredns.a.proxy.ttl=60`). A non-numeric value is ignored. When neither a
  label nor `app.record_ttl` sets a TTL, the field is omitted and CoreDNS
  applies its own default.

---

## Docker

```bash
docker pull ghcr.io/auto-dns/docker-coredns-sync:latest
```

### docker-compose snippet

```yaml
docker-coredns-sync:
  image: ghcr.io/auto-dns/docker-coredns-sync:latest
  restart: unless-stopped
  volumes:
    - /var/run/docker.sock:/var/run/docker.sock:ro
    - ./config.yaml:/config/config.yaml:ro
  environment:
    DOCKER_COREDNS_SYNC_APP_HOSTNAME: homeserver
    DOCKER_COREDNS_SYNC_APP_HOST_IPV4: 192.168.2.10
```

---

## Configuration Overview

Configuration values can be provided via:

1. **Command-line flags** (highest precedence)
2. **Environment variables**
3. **Config file** (`config.yaml` or `config.json`)
4. **Built-in defaults** (lowest precedence)

---

## Configuration Reference

| Flag | Config Key | Env Var | Type | Default | Description |
|------|------------|---------|------|---------|-------------|
| `--app.allowed-record-types` | `app.allowed_record_types` | `DOCKER_COREDNS_SYNC_APP_ALLOWED_RECORD_TYPES` | `[]string` | `["A", "CNAME"]` | DNS record types to allow |
| `--app.docker-label-prefix` | `app.docker_label_prefix` | `DOCKER_COREDNS_SYNC_APP_DOCKER_LABEL_PREFIX` | `string` | `"coredns"` | Docker label namespace |
| `--app.host-ip` | `app.host_ip` | `DOCKER_COREDNS_SYNC_APP_HOST_IP` | `string` | `"127.0.0.1"` | IP to use for A records |
| `--app.hostname` | `app.hostname` | `DOCKER_COREDNS_SYNC_APP_HOSTNAME` | `string` | `"your-hostname"` | Unique logical hostname for this node |
| `--app.poll-interval` | `app.poll_interval` | `DOCKER_COREDNS_SYNC_APP_POLL_INTERVAL` | `int` | `5` | How often to reconcile the registry (in seconds) |
| `--app.record-ttl` | `app.record_ttl` | `DOCKER_COREDNS_SYNC_APP_RECORD_TTL` | `uint` | `0` | Default DNS record TTL in seconds (`0` = unset; CoreDNS uses its own default). Overridable per record via a `coredns.<kind>[.<alias>].ttl` label |
| `--app.heartbeat-ttl` | `app.heartbeat_ttl` | `DOCKER_COREDNS_SYNC_APP_HEARTBEAT_TTL` | `int` | `30` | Lease TTL (seconds) for this host's liveness key. Doubles as the grace period before another host garbage-collects records owned by a host that stopped renewing. `0` or negative disables heartbeats **and** cross-host GC |
| *(config file only)* | `etcd.endpoints` | `DOCKER_COREDNS_SYNC_ETCD_ENDPOINTS` | `[]string` | `["http://localhost:2379"]` | etcd endpoint URLs (supports multiple for cluster) |
| `--etcd.path-prefix` | `etcd.path_prefix` | `DOCKER_COREDNS_SYNC_ETCD_PATH_PREFIX` | `string` | `"/skydns"` | etcd base path |
| `--etcd.lock-ttl` | `etcd.lock_ttl` | `DOCKER_COREDNS_SYNC_ETCD_LOCK_TTL` | `float` | `5.0` | Lock lease time-to-live in seconds |
| `--etcd.lock-timeout` | `etcd.lock_timeout` | `DOCKER_COREDNS_SYNC_ETCD_LOCK_TIMEOUT` | `float` | `2.0` | Lock acquisition timeout |
| `--etcd.lock-retry-interval` | `etcd.lock_retry_interval` | `DOCKER_COREDNS_SYNC_ETCD_LOCK_RETRY_INTERVAL` | `float` | `0.1` | Retry interval for lock acquisition |
| `--log.level` | `log.level` | `DOCKER_COREDNS_SYNC_LOG_LEVEL` | `string` | `"INFO"` | Logging level (`TRACE`, `DEBUG`, `INFO`, `WARN`, `ERROR`, `FATAL`) |

---

## Multi-host Behavior & Record Garbage Collection

Each instance scopes ownership of etcd records by its `app.hostname`. A host only
removes records it owns — except for orphan cleanup described below.

While running, every instance publishes a lease-backed **heartbeat** key (outside
`etcd.path_prefix`, so CoreDNS never sees it) and keeps it alive. The lease TTL is
`app.heartbeat_ttl`. During reconciliation a host treats any record whose owner has
**no live heartbeat** as an orphan and garbage-collects it. The heartbeat lease TTL
is the grace period: a host must be silent for longer than `heartbeat_ttl` before its
records become eligible for removal, so a brief outage or restart will **not** cause
another host to delete its records.

Setting `app.heartbeat_ttl` to `0` (or a negative value) disables both the heartbeat
and cross-host GC, restoring the original conservative behavior (a host only ever
removes its own stale records).

> **Upgrading a multi-host fleet:** roll out this version to all hosts together. A host
> running an older version that doesn't publish a heartbeat will look "dead" to upgraded
> hosts, which would then GC its records.

---

## Config File Locations

Config files are searched in the following paths by default (unless `--config` is passed):

- `$HOME/.config/docker-coredns-sync/config.yaml`
- `/etc/docker-coredns-sync/config.yaml`
- `/config/config.yaml`
- `./config.yaml`

Currently, only the `.yaml` format is explicitly supported unless overriding with a custom config file via the `--config` CLI arg, in which case, the `viper` library will do it's best to infer the file type from its extension.

---

## Example Config File (`config.yaml`)

```yaml
app:
  allowed_record_types:
    - A
    - CNAME
  docker_label_prefix: coredns
  host_ipv4: 192.168.1.100
  host_ipv6: fd20:0:1::100
  hostname: homeserver
  poll_interval: 5
  record_ttl: 0      # 0 = let CoreDNS apply its default; override per record with a .ttl label
  heartbeat_ttl: 30  # liveness lease + cross-host GC grace period; 0 disables

log:
  level: INFO

etcd:
  endpoints:
    - http://192.168.1.10:2379
    - http://192.168.1.11:2379
    - http://192.168.1.12:2379
  path_prefix: /skydns
  lock_ttl: 5.0
  lock_timeout: 2.0
  lock_retry_interval: 0.1
```

---

## Development

### Prerequisites

- Go 1.26.3+
- golangci-lint (optional, for linting)

### Running Tests

```bash
# Run all tests
make test

# Run tests with verbose output
make test-verbose

# Run tests with race detection
make test-race

# Run tests with coverage report
make test-coverage

# Generate HTML coverage report
make test-coverage-html
```

### Or without Make:

```bash
go test ./...
go test -race -coverprofile=coverage.out ./...
go tool cover -func=coverage.out
```

### Linting and Formatting

```bash
make lint     # Run golangci-lint
make format   # Format code with go fmt and goimports
make check    # Run both lint and test
```

### Building

```bash
make build      # Build production Docker image
make build-dev  # Build development Docker image
```

## License

This project is licensed under a [custom MIT-NC License](./LICENSE), which permits non-commercial use only.

You are free to use, modify, and distribute this code for personal, educational, or internal business purposes. **However, commercial use — including bundling with a paid product or service — is strictly prohibited without prior written permission.**

If you are interested in commercial licensing, please contact: [maintainers via GitHub]
