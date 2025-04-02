import docker
import signal
from queue import Queue, Empty
from threading import Thread
from .config import LABEL_PREFIX, HOST_IP, REDIS_PREFIX
from .redis import fqdn_to_zone_and_label, put_record, delete_record, cleanup_stale_records
from .logger import logger

client = docker.from_env()
event_queue = Queue()
shutdown_flag = False

def collect_events():
    logger.debug("Starting Docker event collector thread")
    for event in client.events(decode=True):
        if shutdown_flag:
            break
        if event.get("Type") == "container":
            event_queue.put(event)

def get_container_domains(container):
    labels = container.labels
    if not labels.get(f"{LABEL_PREFIX}.enabled"):
        return []

    records = []
    i = 0
    while True:
        base_key = f"{LABEL_PREFIX}.{i}" if i > 0 else LABEL_PREFIX
        domain_key = f"{base_key}.domain"
        type_key = f"{base_key}.type"
        value_key = f"{base_key}.value"

        if domain_key in labels:
            domain = labels[domain_key]
            record_type = labels.get(type_key, "A").upper()
            record_value = labels.get(value_key, HOST_IP)
            records.append((domain, record_type, record_value))
            i += 1
        else:
            break

    return records

def is_force_enabled(container):
    return container.labels.get(f"{LABEL_PREFIX}.force", "false").lower() == "true"

def register_domains(container, seen_keys=None):
    if container.labels.get(f"{LABEL_PREFIX}.enabled"):
        force = is_force_enabled(container)
        for domain, record_type, value in get_container_domains(container):
            zone, label = fqdn_to_zone_and_label(domain)
            if seen_keys is not None:
                seen_keys.add((f"{REDIS_PREFIX}{zone}", label))
            put_record(domain, value, record_type=record_type, force=force)

def process_start(container):
    register_domains(container)

def process_stop(container):
    domains = get_container_domains(container)
    for domain, record_type, value in domains:
        logger.info(f"Deregistering {domain}")
        delete_record(domain)

def initial_sync():
    seen_keys = set()
    for container in client.containers.list():
        register_domains(container, seen_keys)
    cleanup_stale_records(seen_keys)

def shutdown_handler(signum, frame):
    global shutdown_flag
    shutdown_flag = True
    logger.info("Shutting down docker-etcd-sync...")

signal.signal(signal.SIGINT, shutdown_handler)
signal.signal(signal.SIGTERM, shutdown_handler)

def run():
    logger.info("Starting docker-etcd-sync with live event monitoring")
    Thread(target=collect_events, daemon=True).start()
    initial_sync()
    logger.debug("Processing Docker events")

    while not shutdown_flag:
        try:
            event = event_queue.get(timeout=5)
            status = event.get("status")
            cid = event.get("id")

            if status not in ("start", "die", "destroy"):
                continue

            container = client.containers.get(cid)
            labels = container.labels

            if f"{LABEL_PREFIX}.enabled" not in labels:
                continue

            name = getattr(container, "name", "<unknown>")
            logger.debug(f"Processing {status} for container {name} ({cid[:12]})")

            if status == "start":
                process_start(container)
            elif status in ("die", "destroy"):
                process_stop(container)

        except Empty:
            continue
        except Exception as e:
            if not shutdown_flag:
                logger.error(f"Error processing event: {e}")
