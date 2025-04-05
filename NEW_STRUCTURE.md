# New file structure
```
src/
├── __init__.py
├── main.py                   # App entrypoint: calls SyncEngine
├── config.py                 # Pydantic-based settings loader
├── logger.py                 # Logging setup

# 🧠 Core application logic
├── core/
│   ├── __init__.py
│   ├── dns_record.py         # ARecord, CNAMERecord, base Record model
│   ├── docker_watcher.py     # Listens for Docker events
│   ├── record_builder.py     # Converts container labels → Record[]
│   ├── record_reconciler.py  # Diffs desired vs actual, decides what to change
│   ├── record_validator.py   # Enforces DNS rules (e.g. no CNAME cycles)
│   ├── state.py              # (optional) Tracks active containers, deduping, etc
│   ├── sync_engine.py        # Orchestrates the full sync loop

# 🔌 Interfaces and contracts
├── interfaces/
│   ├── __init__.py
│   ├── registry_interface.py        # DnsRegistry base protocol (CRUD only)
│   ├── registry_with_lock.py        # Optional extension: locked_transaction()

# 💾 Backend implementations
├── backends/
│   ├── __init__.py
│   ├── etcd_registry.py             # Etcd implementation of registry interface

# 🛠 Utility code
├── utils/
│   ├── __init__.py
│   ├── docker_utils.py              # (optional) Docker client helpers
│   ├── errors.py                    # Custom exceptions: ValidationError, RegistryError, etc
│   ├── timing.py                    # (optional) Retry/backoff decorators or helpers

# ❌ To be deleted once refactor is complete
├── backends
│	├── etcd.py
├── core
│	├── record.py
│	├── sync.py
```

# Module Purpose Breakdown (Quick Summary)
| File | Purpose |
|------|---------|
| `main.py` | Entry point: starts the sync loop |
| `sync_engine.py` | High-level logic: runs full reconcile cycle |
| `record_builder.py` | Builds desired records from Docker metadata |
| `record_reconciler.py` | Computes diff between desired and actual |
| `record_validator.py` | Applies DNS-specific validation rules (CNAME loops, etc) |
| `dns_record.py` | Typed DNS record models (via `pydantic.BaseModel`) |
| `registry_interface.py` | CRUD contract for all registries |
| `registry_with_lock.py` | Optional locking for registries that support atomic write validation |
| `etcd_registry.py` | Etcd-specific implementation of the registry |
| `state.py` _(optional)_ | Tracks previously seen container and record state |
| `errors.py` | Custom exceptions for clean error handling |
| `docker_watcher.py` | Subscribes to Docker events (start/stop/etc) |
| `docker_utils.py` _(optional)_ | Helps with safe container inspection or label parsing |