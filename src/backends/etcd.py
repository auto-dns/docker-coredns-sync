import etcd3
import json
import re
import time
from contextlib import contextmanager
from config import load_settings
from logger import setup_logger
from ..core.record import parse_record, is_owned_by, DEFAULT_TTL


logger = setup_logger()
settings = load_settings()

client = etcd3.client(host=settings.etcd_host, port=settings.etcd_port)
LOCK_PREFIX = "__lock__:/"
LOCK_TTL = 3  # seconds
LOCK_RETRY_INTERVAL = 0.1
LOCK_TIMEOUT = 2.0


def fqdn_to_key(fqdn):
    return "/".join(reversed(fqdn.strip(".").split(".")))

def key_to_fqdn(key):
    cleaned_key = re.sub(r'^/skydns/|/x\d+$', '', key).strip('/')
    return ".".join(reversed(cleaned_key.split("/")))

def etcd_full_key(key):
    return f"{settings.etcd_path_prefix.rstrip('/')}/{key}"

def _would_create_cname_cycle(fqdn, target, visited=None, existing_records=None):
    """
    Check if creating a CNAME from fqdn to target would create a cycle.
    
    Args:
        fqdn: The domain name we're creating a CNAME for
        target: The target of the CNAME
        visited: Set of already visited domains (for recursion)
        existing_records: Dictionary of existing records (key -> record) to avoid etcd queries
        
    Returns:
        True if a cycle would be created, False otherwise
    """
    if visited is None:
        visited = set()
    
    # If we've seen this domain before, we have a cycle
    if fqdn in visited:
        return True
    
    # Add current domain to visited set
    visited.add(fqdn)
    
    # If target equals original fqdn, we have a direct cycle
    if target == fqdn:
        return True
    
    # Check if target is another CNAME in our system
    target_key = fqdn_to_key(target)
    target_full_key_prefix = etcd_full_key(target_key)
    
    try:
        # If we have existing records, use them instead of querying etcd
        if existing_records:
            # Look for any keys that match our target prefix
            for key, record in existing_records.items():
                if key.startswith(target_full_key_prefix):
                    if "cname" in record and record["cname"]:
                        # This is a CNAME record, check where it points
                        for cname_entry in record["cname"]:
                            if "host" in cname_entry:
                                # Recursively check if this creates a cycle
                                if _would_create_cname_cycle(target, cname_entry["host"], visited, existing_records):
                                    return True
        else:
            # If we don't have existing records, query etcd
            for raw, meta in client.get_prefix(target_full_key_prefix):
                if raw:
                    record = parse_record(raw.decode())
                    if "cname" in record and record["cname"]:
                        # This is a CNAME record, check where it points
                        for cname_entry in record["cname"]:
                            if "host" in cname_entry:
                                # Recursively check if this creates a cycle
                                if _would_create_cname_cycle(target, cname_entry["host"], visited, existing_records):
                                    return True
    except Exception as e:
        logger.warning(f"Error checking CNAME cycle for {target}: {e}")
    
    return False

@contextmanager
def etcd_lock(key):
    lock_key = f"{LOCK_PREFIX}{key}"
    start = time.time()
    lease = None
    acquired = False

    while time.time() - start < LOCK_TIMEOUT:
        try:
            lease = client.lease(LOCK_TTL)
            success, _ = client.transaction(
                compare=[client.transactions.create(lock_key) == 0],
                success=[client.transactions.put(lock_key, settings.hostname, lease)],
                failure=[],
            )
            if success:
                acquired = True
                break
            else:
                lease.revoke()
        except Exception:
            pass
        time.sleep(LOCK_RETRY_INTERVAL)

    if not acquired:
        logger.warning(f"Timeout acquiring lock for {key}, skipping.")
        yield False
        return

    try:
        yield True
    finally:
        try:
            lease.revoke()
        except Exception:
            pass

# Returns created:bool, existing:bool, conflict:bool, and record_key:string
def put_record(fqdn, value, record_type="A", force=False):
    """
    Add a DNS record to etcd with proper handling of conflicts and record types.
    
    Args:
        fqdn: Fully qualified domain name
        value: IP address for A records, hostname for CNAME records
        record_type: "A" or "CNAME"
        force: Whether to overwrite existing records owned by this host
        
    Returns:
        The etcd key where the record was stored, or None if operation failed
    """
    key = fqdn_to_key(fqdn)
    full_key_prefix = etcd_full_key(key)
    
    # Validate record type
    if record_type not in ["A", "CNAME"]:
        logger.warning(f"Unsupported record type: {record_type}")
        return False, False, False, None
    
    with etcd_lock(key) as acquired:
        if not acquired:
            return False, False, False, None
        
        try:
            # Get all existing records for this domain
            existing_records = {}  # Maps etcd key to record content
            existing_a_records = []
            existing_cname_records = []
            next_index = 1
            
            # Collect all existing records and determine next available index
            existing_indices = set()
            for raw, meta in client.get_prefix(full_key_prefix):
                if raw:
                    record_key = meta.key.decode()

                    record_fqdn = key_to_fqdn(record_key)
                    if record_fqdn != fqdn:
                        continue

                    record = parse_record(raw.decode())

                    if (record.get("record_type") == record_type and
                        record_type in ('A', 'CNAME') and
                        record.get("host") == value and
                        record.get("owner") == settings.hostname):
                        logger.info(f"Found {record_type} record for {fqdn} -> {value} owned by this host ({settings.hostname})")
                        return False, True, False, record_key
                    
                    existing_records[record_key] = record

                    # Track record types
                    existing_record_type = record.get('record_type', None)
                    if existing_record_type == "A":
                        existing_a_records.append((record_key, record))
                    elif existing_record_type == "CNAME":
                        existing_cname_records.append((record_key, record))
                    else:
                        # TODO: Update this to check - does record.host look like an IP address? If so, A. If not, CNAME.
                        continue
                    
                    # Extract index from key to track next available index
                    key_parts = record_key.split('/')
                    if key_parts[-1].startswith('x'):
                        try:
                            index = int(key_parts[-1][1:])
                            existing_indices.add(index)
                        except ValueError:
                            pass
            
            # Get lowest next index available
            next_index = 1
            while next_index in existing_indices:
                next_index += 1

            # For CNAME records, check for cycles within the lock
            if record_type == "CNAME":
                if _would_create_cname_cycle(fqdn, value, existing_records=existing_records):
                    logger.error(f"Cannot create CNAME record that would result in a cycle: {fqdn} -> {value}")
                    return False, False, True, None
            
            # Check constraints based on record type
            if record_type == "A":
                # A record: can't add if CNAME exists
                if existing_cname_records:
                    logger.warning(f"Cannot add A record for {fqdn} because CNAME record exists")
                    return False, False, True, None
                
                # Check if we already have an A record owned by this host
                for key, record in existing_a_records:
                    if is_owned_by(record, settings.hostname):
                        # We found our own record
                        if force:
                            # Update existing record
                            new_record = record.copy()
                            new_record["a"] = [{"ttl": DEFAULT_TTL, "host": value}]
                            client.put(key, json.dumps(new_record))
                            logger.info(f"Updated existing A record for {fqdn} -> {value}")
                            return True, True, False, key
                        else:
                            logger.warning(f"A record for {fqdn} already exists and force=False")
                            return False, True, False, key
            
            elif record_type == "CNAME":
                # CNAME record: can't add if any A or CNAME exists
                if existing_a_records or existing_cname_records:
                    logger.warning(f"Cannot add CNAME record for {fqdn} because A or CNAME records exist")
                    # TODO: this may validly be due to it already existing rather than conflict
                    return False, False, True, None
            
            # If we get here, we're creating a new record
            new_key = f"{full_key_prefix}/x{next_index}"
            
            # Create the record based on type
            new_record = {"record_type": record_type, "owner": settings.hostname}

            if record_type in ("A", "CNAME"):
                new_record["host"] = value
            elif record_type == "TXT":
                new_record["text"] = value
            
            # Store the record
            client.put(new_key, json.dumps(new_record))
            logger.info(f"Added {record_type.upper()} record for {fqdn} -> {value}")
            return True, False, False, new_key
            
        except Exception as e:
            logger.error(f"Failed to put record for {fqdn}: {e}")
            return False, False, False, None

def delete_record(fqdn, value=None, record_type="A"):
    """
    Delete DNS records for a given FQDN.
    
    Args:
        fqdn: Fully qualified domain name
        value: Optional value to match (IP for A records, hostname for CNAME)
        record_type: Record type to delete
    """
    record_type = record_type
    key = fqdn_to_key(fqdn)
    full_key_prefix = etcd_full_key(key)

    with etcd_lock(key) as acquired:
        if not acquired:
            return

        try:
            for raw, meta in client.get_prefix(full_key_prefix):
                if raw:
                    record_key = meta.key.decode()
                    record = parse_record(raw.decode())
                    
                    # Only delete records owned by this host
                    if is_owned_by(record, settings.hostname):
                        # If value is specified, only delete records with matching value
                        if value is not None:
                            if record_type == "A":
                                if record.get("host") == value:
                                    client.delete(record_key)
                                    logger.info(f"Deleted record {record_key} for {fqdn}")
                                    return
                            elif record_type == "CNAME":
                                if record.get("host") == value:
                                    client.delete(record_key)
                                    logger.info(f"Deleted record {record_key} for {fqdn}")
                                    return
            logger.debug(f"No matching records found to delete for {fqdn}")    
        except Exception as e:
            logger.error(f"Error deleting record {fqdn}: {e}")

def cleanup_stale_records(running_container_keys):
    try:
        prefix = settings.etcd_path_prefix.rstrip("/") + "/"
        for value, meta in client.get_prefix(prefix):
            key = meta.key.decode()
            logger.debug(f"Checking etcd key: {key}")
            if key not in running_container_keys:
                try:
                    record = parse_record(value)
                    if is_owned_by(record, settings.hostname):
                        logger.warning(f"Removing stale entry {key}")
                        client.delete(meta.key)
                except Exception as e:
                    logger.warning(f"Skipping corrupt or invalid entry {key}: {e}")
    except Exception as e:
        logger.error(f"Error during cleanup: {e}")
