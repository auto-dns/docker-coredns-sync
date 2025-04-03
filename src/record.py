import json
from .logger import logger

DEFAULT_TTL = 300

def parse_record(json_str):
    try:
        return json.loads(json_str)
    except Exception as e:
        logger.error(f"Failed to parse record: {e}")
        return {}

def is_owned_by(record, hostname):
    return record.get("owner") == hostname

def fqdn_to_zone_and_label(fqdn):
    fqdn = fqdn.rstrip(".")
    parts = fqdn.split(".")
    for i in range(1, len(parts)):
        zone = ".".join(parts[i:]) + "."
        label = ".".join(parts[:i])
        return zone, label
    return fqdn + ".", "@"