import json
import time
from contextlib import contextmanager
import etcd3

from .config import ETCD_HOST, ETCD_PATH_PREFIX, HOSTNAME
from .logger import logger

client = etcd3.client(host=ETCD_HOST, port=2379)

DEFAULT_TTL = 300
LOCK_TTL_SEC = 3
LOCK_RETRY_INTERVAL = 0.1  # 100ms
LOCK_TIMEOUT = 2.0  # seconds
MAX_CYCLE_DEPTH = 10

def fqdn_to_key(fqdn):
    parts = fqdn.strip(".").split(".")
    parts.reverse()
    return "/".join(parts)

def etcd_full_key(fqdn):
    return f"{ETCD_PATH_PREFIX}/{fqdn_to_key(fqdn)}"

@contextmanager
def etcd_lock(fqdn):
    key = f"__lock__:{fqdn}"
    lock = client.lock(key, ttl=LOCK_TTL_SEC)
    try:
        acquired = lock.acquire(timeout=LOCK_TIMEOUT)
        if not acquired:
            logger.warning(f"Timeout acquiring lock for {fqdn}, skipping.")
            yield False
            return
        yield True
    finally:
        try:
            lock.release()
        except Exception:
            pass

def detect_cycle(fqdn, target, visited=None, depth=0):
    if visited is None:
        visited = set()
    if fqdn == target or target in visited:
        return True
    if depth > MAX_CYCLE_DEPTH:
        logger.warning(f"Cycle detection exceeded max depth for {fqdn}, treating as cycle.")
        return True

    visited.add(target)
    try:
        val, _ = client.get(etcd_full_key(target))
        if val:
            record = json.loads(val)
            for t in (record.get("cname") or []):
                if detect_cycle(fqdn, t["host"], visited, depth + 1):
                    return True
            for a in (record.get("a") or []):
                if detect_cycle(fqdn, target, visited, depth + 1):
                    return True
    except Exception as e:
        logger.warning(f"Error during cycle detection for {fqdn}: {e}")
    return False

def put_record(fqdn, value, record_type="A", force=False):
    key = etcd_full_key(fqdn)
    record_type = record_type.lower()

    with etcd_lock(fqdn) as acquired:
        if not acquired:
            return
        try:
            existing_val, _ = client.get(key)
            if existing_val:
                existing = json.loads(existing_val)
                if existing.get("owner") != HOSTNAME and not force:
                    logger.warning(f"{fqdn} is already owned by {existing.get('owner')}, skipping.")
                    return
                if record_type in existing and not force:
                    logger.warning(f"{fqdn} already has a {record_type.upper()} record, skipping.")
                    return
                if record_type == "cname" and "a" in existing and not force:
                    logger.warning(f"{fqdn} already has an A record, cannot add CNAME, skipping.")
                    return
                if record_type == "a" and "cname" in existing and not force:
                    logger.warning(f"{fqdn} already has a CNAME record, cannot add A, skipping.")
                    return
        except Exception as e:
            logger.error(f"Error checking record {fqdn}: {e}")
            return

        if record_type == "cname" and detect_cycle(fqdn, value):
            logger.error(f"Cycle detected for CNAME {fqdn} → {value}, skipping.")
            return

        logger.info(f"Registering {fqdn} [{record_type}] → {value}")
        record = {
            record_type: [{"ttl": DEFAULT_TTL, "ip": value}] if record_type == "a" else [{"ttl": DEFAULT_TTL, "host": value}],
            "owner": HOSTNAME
        }
        client.put(key, json.dumps(record))

def delete_record(fqdn):
    key = etcd_full_key(fqdn)
    with etcd_lock(fqdn) as acquired:
        if not acquired:
            return
        try:
            val, _ = client.get(key)
            if val:
                data = json.loads(val)
                if data.get("owner") == HOSTNAME:
                    logger.info(f"Deleting owned record: {fqdn}")
                    client.delete(key)
                else:
                    logger.debug(f"Skipping delete: {fqdn} owned by {data.get('owner')}")
        except Exception as e:
            logger.error(f"Error checking ownership for {fqdn}: {e}")

def cleanup_stale_records(seen_keys):
    prefix = ETCD_PATH_PREFIX.rstrip("/") + "/"
    try:
        for value, metadata in client.get_prefix(prefix):
            key = metadata.key.decode().replace(prefix, "")
            if f'"owner":"{HOSTNAME}"' in value.decode():
                if key not in seen_keys:
                    logger.warning(f"Removing stale entry {key}")
                    client.delete(metadata.key)
    except Exception as e:
        logger.error(f"Error fetching records for cleanup: {e}")