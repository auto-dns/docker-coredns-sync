
from typing import Dict, Iterable, List, Set, Tuple

from src.config import load_settings
from src.core.dns_record import ARecord, CNAMERecord
from src.core.record_intent import RecordIntent
from src.core.record_validator import validate_record
from src.logger import logger

settings = load_settings()


def _record_key(intent: RecordIntent) -> Tuple[str, str, str]:
    return (intent.record.name, intent.record.record_type, str(intent.record.value))


def reconcile(
    desired: Iterable[RecordIntent], actual: Iterable[RecordIntent]
) -> Tuple[List[RecordIntent], List[RecordIntent]]:
    logger.debug("[reconciler] Starting unified reconciliation")

    desired_by_key: Dict[Tuple[str, str, str], RecordIntent] = {}
    actual_by_key: Dict[Tuple[str, str, str], RecordIntent] = {_record_key(r): r for r in actual}
    to_add: Dict[Tuple[str, str, str], RecordIntent] = {}
    to_remove: Dict[Tuple[str, str, str], RecordIntent] = {}

    # Select the best contender for each key
    for r in desired:
        key = _record_key(r)
        existing = desired_by_key.get(key)
        if (
            not existing
            or r.force
            or (not existing.force and r.created > existing.created)
        ):
            desired_by_key[key] = r

    # Reconcile each contender
    for key, desired_r in desired_by_key.items():
        actual_r = actual_by_key.get(key)

        # 1. Skip if already owned by us
        if actual_r and actual_r.hostname == desired_r.hostname and actual_r.container_name == desired_r.container_name:
            logger.debug(
                f"[reconciler] Skipping record already owned by us: "
                f"{desired_r.record.render()} (container: {desired_r.container_name})"
            )
            continue

        # 2. Skip if actual exists and has precedence
        if actual_r and not desired_r.force and desired_r.created < actual_r.created:
            logger.warning(
                f"[reconciler] Record conflict (not overriding): {desired_r.record.render()} "
                f"already owned by {actual_r.hostname}/{actual_r.container_name}"
            )
            continue

        # 3. Compute evictions if we override
        evictions: Dict[Tuple[str, str, str], RecordIntent] = {}

        # If we made it to this point, it's because we want to override actual_r
        # because we're skipping (continue) if we don't want to override, and it should exist at this point
        if actual_r:
            evictions[_record_key(actual_r)] = actual_r

        if isinstance(desired_r.record, CNAMERecord):
            for r in actual:
                if r.record.name == desired_r.record.name:
                    evictions[_record_key(r)] = r

        # 4. Simulate state (actual + to_add - to_remove - evictions)
        simulated_state = list(actual_by_key.values())
        simulated_state += list(to_add.values())
        key_to_remove = set(to_remove.keys()) | set(evictions.keys())
        simulated_state = [r for r in simulated_state if _record_key(r) not in key_to_remove]

        # 5. Validate
        try:
            validate_record(desired_r, simulated_state)
            logger.info(
                f"[reconciler] Adding new record: {desired_r.record.render()} "
                f"(owned by {desired_r.hostname}/{desired_r.container_name})"
            )
            to_add[key] = desired_r
            to_remove.update(evictions)
        except Exception as e:
            # TODO: do we want to log this every time, or could it benefit from the "log to warn first time and then log to debug next time we get a duplicate key"?
            logger.warning(
                f"[reconciler] Skipping invalid record {desired_r.record.render()} — {e}"
            )

    # Final stale cleanup (only remove what we own and don't plan to keep)
    all_desired_keys = set(desired_by_key.keys()) | set(to_add.keys())
    for r in actual:
        if r.hostname == settings.hostname and _record_key(r) not in all_desired_keys:
            to_remove[_record_key(r)] = r

    return list(to_add.values()), list(to_remove.values())
