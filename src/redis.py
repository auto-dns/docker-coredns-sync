import json
import redis
import time
from contextlib import contextmanager
from .config import REDIS_HOST, REDIS_PORT, REDIS_PASSWORD, REDIS_PREFIX, HOSTNAME
from .logger import logger

client = redis.Redis(host=REDIS_HOST, port=REDIS_PORT, password=REDIS_PASSWORD, decode_responses=True)

DEFAULT_TTL = 300
LOCK_TTL_MS = 3000  # 3 seconds
LOCK_RETRY_INTERVAL = 0.1  # 100 ms
LOCK_TIMEOUT = 2.0  # seconds
LOCK_PREFIX = "__lock__:/"
MAX_CYCLE_DEPTH = 10

@contextmanager
def redis_lock(zone, label):
    lock_key = f"{LOCK_PREFIX}{zone}:{label}"
    start = time.time()
    acquired = False

    while time.time() - start < LOCK_TIMEOUT:
        if client.set(lock_key, HOSTNAME, nx=True, px=LOCK_TTL_MS):
            acquired = True
            break
        time.sleep(LOCK_RETRY_INTERVAL)

    if not acquired:
        logger.warning(f"Timeout acquiring lock for {zone}:{label}, skipping.")
        yield False
        return

    try:
        yield True
    finally:
        client.delete(lock_key)

def fqdn_to_zone_and_label(fqdn):
    fqdn = fqdn.rstrip(".")
    parts = fqdn.split(".")
    for i in range(1, len(parts)):
        zone = ".".join(parts[i:]) + "."
        label = ".".join(parts[:i])
        return zone, label
    return fqdn + ".", "@"

def detect_cycle(fqdn, target, visited=None, depth=0):
    if visited is None:
        visited = set()
    if fqdn == target or target in visited:
        return True
    if depth > MAX_CYCLE_DEPTH:
        logger.warning(f"Cycle detection exceeded max depth for {fqdn}, treating as cycle.")
        return True

    visited.add(target)
    zone, label = fqdn_to_zone_and_label(target)
    redis_zone = REDIS_PREFIX + zone
    try:
        val = client.hget(redis_zone, label)
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
    zone, label = fqdn_to_zone_and_label(fqdn)
    redis_zone = REDIS_PREFIX + zone
    record_type = record_type.lower()

    with redis_lock(zone, label) as acquired:
        if not acquired:
            return
        try:
            existing_json = client.hget(redis_zone, label)
            if existing_json:
                existing = json.loads(existing_json)
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
        client.hset(redis_zone, label, json.dumps(record))

def delete_record(fqdn):
    zone, label = fqdn_to_zone_and_label(fqdn)
    redis_zone = REDIS_PREFIX + zone
    with redis_lock(zone, label) as acquired:
        if not acquired:
            return
        try:
            val = client.hget(redis_zone, label)
            if val:
                data = json.loads(val)
                if data.get("owner") == HOSTNAME:
                    logger.info(f"Deleting owned record: {fqdn}")
                    client.hdel(redis_zone, label)
                else:
                    logger.debug(f"Skipping delete: {fqdn} owned by {data.get('owner')}")
        except Exception as e:
            logger.error(f"Error checking ownership for {fqdn}: {e}")

def cleanup_stale_records(seen_keys):
    try:
        for key in client.scan_iter(f"{REDIS_PREFIX}*"):
            fields = client.hkeys(key)
            for field in fields:
                with redis_lock(key, field) as acquired:
                    if not acquired:
                        continue
                    val = client.hget(key, field)
                    if val and f'"owner":"{HOSTNAME}"' in val:
                        if (key, field) not in seen_keys:
                            logger.warning(f"Removing stale entry {field} in zone {key}")
                            client.hdel(key, field)
    except Exception as e:
        logger.error(f"Error fetching records for cleanup: {e}")
