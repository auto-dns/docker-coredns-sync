import docker
import signal
import time
from queue import Queue, Empty
from threading import Thread
from config import load_settings
from ..backends.etcd import put_record, delete_record, cleanup_stale_records, fqdn_to_key
from ..logger import logger

settings = load_settings()

client = docker.from_env()
event_queue = Queue()
shutdown_flag = False

# Store container information for later use
container_cache = {}

# Container utility functions
def is_enabled(container):
    return container.labels.get(f"{settings.docker_label_prefix}.enabled", "false").lower() == "true"

def is_force_enabled(container):
    return container.labels.get(f"{settings.docker_label_prefix}.force", "false").lower() == "true"

# Cache container information for later use
def cache_container(container_id, container):
    if is_enabled(container):
        # Store only the necessary information we need for cleanup
        container_cache[container_id] = {
            'name': container.name,
            'domains': get_container_domains(container)
        }
        logger.debug(f"Cached container {container.name} ({container_id[:12]})")

# Filter and collect docker events and put them in the event queue
def collect_events():
    logger.debug("Starting Docker event collector thread")
    for event in client.events(decode=True):
        if shutdown_flag:
            break
        # Filter out unhandled statuses
        if event.get("Type") == "container" and event.get("status") in ("start", "die", "destroy"):
            try:
                # Check if the container has our labels before adding to processing queue
                cid = event.get("id")
                container = client.containers.get(cid)

                if is_enabled(container):
                    cache_container(cid, container)
                    logger.debug(f"Queueing {event.get('status')} event for container {container.name} ({cid[:12]})")
                    event_queue.put(event)
            except docker.errors.NotFound:
                logger.debug(f"Container {cid[:12]} not found, skipping")
            except Exception as e:
                logger.error(f"Error pre-processing event: {e}")

def get_container_domains(container):
    labels = container.labels
    if not labels.get(f"{settings.docker_label_prefix}.enabled", "false").lower() == "true":
        return []

    records = []
    record_prefixes = set()
    
    # Find all unique record identifiers (for both formats)
    for key in labels.keys():
        parts = key.split('.')
        
        # Format 1: prefix.record_type.field_name (for single records)
        if len(parts) == 3 and parts[0] == settings.docker_label_prefix and parts[2] in ["name", "value"]:
            record_type = parts[1]
            record_prefixes.add(f"{settings.docker_label_prefix}.{record_type}")
        
        # Format 2: prefix.record_type.identifier.field_name (for multiple records)
        elif len(parts) >= 4 and parts[0] == settings.docker_label_prefix and parts[3] in ["name", "value"]:
            record_type = parts[1]
            identifier = parts[2]
            record_prefixes.add(f"{settings.docker_label_prefix}.{record_type}.{identifier}")
    
    # Process each unique record
    for prefix in record_prefixes:
        name_key = f"{prefix}.name"
        value_key = f"{prefix}.value"
        
        # Skip if name is missing
        if name_key not in labels:
            logger.warning(f"Record {prefix} missing name field, skipping")
            continue
            
        name = labels[name_key]
        
        # Extract record type from prefix
        prefix_parts = prefix.split('.')
        record_type = prefix_parts[1].upper()
        
        if record_type == "A":
            # For A records, value is optional (defaults to HOST_IP)
            record_value = labels.get(value_key, settings.host_ip)
            records.append((name, record_type, record_value))
        elif record_type == "CNAME":
            # For CNAME records, value is required
            if value_key in labels:
                record_value = labels[value_key]
                records.append((name, record_type, record_value))
            else:
                logger.warning(f"CNAME record {prefix} missing required value field, skipping")
        else:
            logger.warning(f"Unsupported record type: {record_type} for {prefix}, skipping")
    
    return records


# Register a single domain
def register_domain(domain, value, record_type="A", force=False):
    logger.debug(f"Registering {domain}")
    return put_record(domain, value, record_type=record_type, force=force)

# Register all domains for a container
def register_domains(container):
    found_domain_keys = set()
    force = is_force_enabled(container)
    for domain, record_type, value in get_container_domains(container):
        created, existing, conflict, record_key = register_domain(domain, value, record_type=record_type, force=force)
        if created or existing:
            found_domain_keys.add(record_key)
    return found_domain_keys

# Handle shutting down gracefully
def shutdown_handler(signum, frame):
    global shutdown_flag
    shutdown_flag = True
    logger.info("Shutting down docker-etcd-sync...")

# Register signal handling
signal.signal(signal.SIGINT, shutdown_handler)
signal.signal(signal.SIGTERM, shutdown_handler)

# Handle initial synchronization
def initial_sync():
    running_container_keys = set()
    for container in client.containers.list():
        if is_enabled(container):
            cache_container(container.id, container)
            found_domain_keys = register_domains(container)
            running_container_keys.update(found_domain_keys)

    logger.debug(f"Seen keys before cleanup: {running_container_keys}")
    cleanup_stale_records(running_container_keys)

# Handle "start" events
def process_start(container):
    register_domains(container)

# Handle "stop" and "die" events
def process_stop(container):
    domains = get_container_domains(container)
    for domain, record_type, value in domains:
        logger.info(f"Deregistering {domain} for IP {value}")
        delete_record(domain, value=value, record_type=record_type)

def run():
    logger.info("Starting docker-etcd-sync with live event monitoring")
    Thread(target=collect_events, daemon=True).start()
    time.sleep(0.5)
    initial_sync()
    logger.debug("Processing Docker events")

    while not shutdown_flag:
        try:
            event = event_queue.get(timeout=5)
            status = event.get("status")
            cid = event.get("id")

            try:
                container = client.containers.get(cid)
                name = getattr(container, "name", "<unknown>")
                logger.debug(f"Processing {status} for container {name} ({cid[:12]})")
            
                if status == "start":
                    process_start(container)
                elif status in ("die", "destroy"):
                    process_stop(container)
                else:
                    logger.debug(f"Ignoring unhandled event: {status}")
            except docker.errors.NotFound:
                logger.warning(f"Container {cid[:12]} not found, likely already removed")
                # Use cached container information for cleanup if available
                if cid in container_cache and status in ("die", "destroy"):
                    cached_info = container_cache[cid]
                    logger.info(f"Using cached info to deregister domains for {cached_info['name']} ({cid[:12]})")
                    process_stop(cached_info['domains'])
                    # Remove from cache after processing
                    del container_cache[cid]
                else:
                    logger.warning(f"No cached information for container {cid[:12]}, cannot clean up DNS records")

        except Empty:
            continue
        except Exception as e:
            if not shutdown_flag:
                logger.error(f"Error processing event: {e}")
