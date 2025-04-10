
from collections import defaultdict
from typing import Dict, Iterable, List, Tuple

from src.config import load_settings
from src.core.dns_record import ARecord, CNAMERecord
from src.core.record_intent import RecordIntent
from src.core.record_validator import validate_record
from src.logger import logger

settings = load_settings()


def _record_key(r: RecordIntent) -> Tuple[str, str, str]:
    return (r.record.name, r.record.record_type, str(r.record.value))

def _should_replace_existing(new: RecordIntent, existing: RecordIntent) -> bool:
    """
    Returns True if `new` should take precedence over `existing`.
    """
    if new.force and not existing.force:
        return True
    if not new.force and existing.force:
        return False
    return new.created < existing.created

def filter_record_intents(records: Iterable[RecordIntent]) -> Tuple[List[RecordIntent]]:
    logger.debug("[reconciler] Reconciling desired records against each other")

    desired_by_name_type: Dict[str, Dict[str, Dict[str, RecordIntent]]] = defaultdict(lambda: defaultdict(dict))

    for r in records:
        name = r.record.name
        value = str(r.record.value)

        existing_types = desired_by_name_type.get(name, {})
        existing_a = existing_types.get("A", {})
        existing_cname = existing_types.get("CNAME", {})

        # TODO: add warning logs if a record is overridden for one reason or another
        if isinstance(r.record, ARecord):
            if existing_cname:
                # Fetch the CNAME record (we don't have its value, so we use next(iter()) on the dict to fetch the only one) to reference its "force" and "created" fields
                existing = next(iter(existing_cname.values()))
                if _should_replace_existing(r, existing):
                    # Remove CNAME and add this A
                    del desired_by_name_type[name]["CNAME"]
                    desired_by_name_type[name]["A"][value] = r
            elif value in existing_a:
                existing = existing_a[value]
                if _should_replace_existing(r, existing):
                    desired_by_name_type[name]["A"][value] = r
            else:
                # No conflict, just add
                desired_by_name_type[name]["A"][value] = r

        elif isinstance(r.record, CNAMERecord):
            if existing_a:
                # There are A records with the same name
                # Only need to inspect one (they’re all same name)
                existing = next(iter(existing_a.values()))
                if _should_replace_existing(r, existing):
                    # Remove all A records and add CNAME
                    del desired_by_name_type[name]["A"]
                    desired_by_name_type[name]["CNAME"][value] = r
            elif existing_cname:
                existing = next(iter(existing_cname.values()))
                if _should_replace_existing(r, existing):
                    # Replace CNAME with this one
                    desired_by_name_type[name]["CNAME"][value] = r
            else:
                # No conflict, just add
                desired_by_name_type[name]["CNAME"][value] = r
    
    reconciled_intents = [
        intent
        for record_type_map in desired_by_name_type.values()
        for value_map in record_type_map.values()
        for intent in value_map.values()
    ]

    return reconciled_intents
    

def reconcile_and_validate(
    desired: Iterable[RecordIntent], actual: Iterable[RecordIntent]
) -> Tuple[List[RecordIntent], List[RecordIntent]]:
    logger.debug("[reconciler] Starting unified reconciliation")

    actual_by_name_type: Dict[str, Dict[str, Dict[str, RecordIntent]]] = defaultdict(lambda: defaultdict(dict))
    for r in actual:
        name = r.record.name
        record_type = r.record.record_type
        value = str(r.record.value)
        actual_by_name_type[name][record_type][value] = r
    
    to_add: Dict[Tuple[str, str, str], RecordIntent] = {}
    to_remove: Dict[Tuple[str, str, str], RecordIntent] = {}

    # Reconcile each contender
    for desired_record in desired:
        name = desired_record.record.name
        value = str(desired_record.record.value)

        actual_types = actual_by_name_type.get(name, {})
        actual_a = actual_types.get("A", {})
        actual_cname = actual_types.get("CNAME", {})

        evictions: Dict[Tuple[str, str, str], RecordIntent] = {}

        # Loop over each desired local record
        # If we detect that it shouldn't 
        if isinstance(desired_record.record, ARecord):
            # Local record intention is an A record
            if actual_cname:
                # Actual remote record is a CNAME record
                actual_record = next(iter(actual_cname.values()))
                if desired_record.force:
                    logger.warning(f"[reconciler] Record conflict between local {desired_record.record.render()} and remote {' / '.join([r.record.render() for r in actual_cname.values()])} - evicting remote due force container label")
                    # Forcibly evict all CNAMEs to enforce only one CNAME per hostname
                    evictions.update({_record_key(r): r for r in actual_cname})
                elif desired_record.created < actual_record.created:
                    logger.warning(f"[reconciler] Record conflict between local {desired_record.record.render()} and remote {' / '.join([r.record.render() for r in actual_cname.values()])} - evicting remote due to container age")
                    # Forcibly evict all CNAMEs to enforce only one CNAME per hostname
                    evictions.update({_record_key(r): r for r in actual_cname})
                else:
                    # We're not evicting, so skip the rest for this record
                    continue
            elif value in actual_a:
                # Actual remote record is an A record
                actual_record = actual_a[value]
                if actual_record == desired_record:
                    continue
                elif desired_record.force:
                    logger.warning(f"[reconciler] Record conflict between local {desired_record.record.render()} and remote {actual_record.record.render()} - evicting remote due to force container label")
                    evictions[_record_key(actual_record)] = actual_record
                elif desired_record.created < actual_record.created:
                    logger.warning(f"[reconciler] Record conflict between local {desired_record.record.render()} and remote {actual_record.record.render()} - evicting remote due to container age")
                    evictions[_record_key(actual_record)] = actual_record
                else:
                    # We're not evicting, so skip the rest for this record
                    continue
            # Else: Don't skip - just add with no evictions - no need for an else statement, this will just work
        
        elif isinstance(desired_record.record, CNAMERecord):
            # Local record intention is a CNAME
            if actual_a:
                # Actual remote record is an A record
                if desired_record.force:
                    logger.warning(f"[reconciler] Record conflict between local {desired_record.record.render()} and remote {' / '.join([r.record.render() for r in actual_a.values()])} - evicting remote due to force container label")
                    # Evict all A records
                    evictions.update({_record_key(r): r for r in actual_a})
                elif all([desired_record.created < r.created for r in actual_a.values()]):
                    # Desired local record is older than all remote records
                    logger.warning(f"[reconciler] Record conflict between local {desired_record.record.render()} and remote {' / '.join([r.record.render() for r in actual_a.values()])} - evicting remote due to container age")
                    # Evict all A records
                    evictions.update({_record_key(r): r for r in actual_a})
                else:
                    # We're not evicting, so skip the rest for this record
                    continue
            elif actual_cname:
                actual_record = next(iter(actual_cname.values()))
                if actual_record == desired_record:
                    continue
                elif desired_record.force:
                    logger.warning(f"[reconciler] Record conflict between local {desired_record.record.render()} and remote {' / '.join([r.record.render() for r in actual_cname.values()])} - evicting remote due to force container label")
                    # Forcibly evict all CNAMEs to enforce only one CNAME per hostname
                    evictions.update({_record_key(r): r for r in actual_cname})
                elif desired_record.created < actual_record.created:
                    logger.warning(f"[reconciler] Record conflict between local {desired_record.record.render()} and remote {' / '.join([r.record.render() for r in actual_a.values()])} - evicting remote due to container age")
                    # Forcibly evict all CNAMEs to enforce only one CNAME per hostname
                    evictions.update({_record_key(r): r for r in actual_a})
                else:
                    # We're not evicting, so skip the rest for this record
                    continue
                # Else: Don't skip - just add with no evictions - no need for an else statement, this will just work

        # Simulate state (actual + to_add - to_remove - evictions)
        simulated_state = actual.copy()
        simulated_state += list(to_add.values())
        key_to_remove = set(to_remove.keys()) | set(evictions.keys())
        simulated_state = [r for r in simulated_state if _record_key(r) not in key_to_remove]

        # 5. Validate
        try:
            validate_record(desired_record, simulated_state)
            logger.info(
                f"[reconciler] Adding new record: {desired_record.record.render()} "
                f"(owned by {desired_record.hostname}/{desired_record.container_name})"
            )
            to_add[_record_key(desired_record)] = desired_record
            to_remove.update(evictions)
        except Exception as e:
            # TODO: do we want to log this every time, or could it benefit from the "log to warn first time and then log to debug next time we get a duplicate key"?
            logger.warning(
                f"[reconciler] Skipping invalid record {desired_record.record.render()} — {e}"
            )

    # Final stale cleanup (only remove what we own and don't plan to keep)
    all_desired_keys = set([_record_key(r) for r in desired]) | set(to_add.keys())
    for r in actual:
        if r.hostname == settings.hostname and _record_key(r) not in all_desired_keys:
            to_remove[_record_key(r)] = r

    return list(to_add.values()), list(to_remove.values())
