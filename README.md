# docker-coredns-sync

`docker-coredns-sync` listens for Docker container events and automatically registers/deregisters A and CNAME records in etcd using the [SkyDNS/CoreDNS etcd plugin format](https://coredns.io/plugins/etcd/). This enables dynamic DNS resolution for containers via CoreDNS.

---

## Features

- Supports **A** and **CNAME** records
- Multiple domain support per container
- Prevents CNAME cycles
- Automatically removes stale records
- Graceful shutdown support
- Flexible configuration via **flags**, **env vars**, and **config file**
- Supports both **YAML** and **JSON** config formats

---

## Docker Label Configuration

### Required

- `coredns.enabled=true` — Enables DNS registration for this container.

### A Record

- `coredns.a.name=foo.example.com`
- `coredns.a.value=192.168.200.1` *(optional, defaults to `host_ip`)*

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
- `coredns.a.name.force=true` — Force a specific A record

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
| `--etcd.host` | `etcd.host` | `DOCKER_COREDNS_SYNC_ETCD_HOST` | `string` | `"localhost"` | etcd host |
| `--etcd.port` | `etcd.port` | `DOCKER_COREDNS_SYNC_ETCD_PORT` | `int` | `2379` | etcd port |
| `--etcd.path-prefix` | `etcd.path_prefix` | `DOCKER_COREDNS_SYNC_ETCD_PATH_PREFIX` | `string` | `"/skydns"` | etcd base path |
| `--etcd.lock-ttl` | `etcd.lock_ttl` | `DOCKER_COREDNS_SYNC_ETCD_LOCK_TTL` | `float` | `5.0` | Lock lease time-to-live in seconds |
| `--etcd.lock-timeout` | `etcd.lock_timeout` | `DOCKER_COREDNS_SYNC_ETCD_LOCK_TIMEOUT` | `float` | `2.0` | Lock acquisition timeout |
| `--etcd.lock-retry-interval` | `etcd.lock_retry_interval` | `DOCKER_COREDNS_SYNC_ETCD_LOCK_RETRY_INTERVAL` | `float` | `0.1` | Retry interval for lock acquisition |
| `--log.level` | `log.level` | `DOCKER_COREDNS_SYNC_LOG_LEVEL` | `string` | `"INFO"` | Logging level (`TRACE`, `DEBUG`, `INFO`, `WARN`, `ERROR`, `FATAL`) |

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
  host_ip: 192.168.1.100
  hostname: mozart
  poll_interval: 5

log:
  level: INFO

etcd:
  host: etcd
  port: 2379
  path_prefix: /skydns
  lock_ttl: 5.0
  lock_timeout: 2.0
  lock_retry_interval: 0.1
```

## License

This project is licensed under a [custom MIT-NC License](./LICENSE), which permits non-commercial use only.

You are free to use, modify, and distribute this code for personal, educational, or internal business purposes. **However, commercial use — including bundling with a paid product or service — is strictly prohibited without prior written permission.**

If you are interested in commercial licensing, please contact: [auto-dns@sl.carroll.live]
