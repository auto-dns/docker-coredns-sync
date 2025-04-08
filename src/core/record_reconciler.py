from typing import Dict, Iterable, List, Tuple

from src.config import load_settings
from src.core.record_intent import RecordIntent
from src.logger import logger

settings = load_settings()


def _intent_key(intent: RecordIntent) -> Tuple[str, str, str]:
    """Create a deduplication key based on record name, type, and value."""
    return (intent.record.name, intent.record.record_type, str(intent.record.value))


def reconcile_additions(
    desired: Iterable[RecordIntent], actual: Iterable[RecordIntent]
) -> List[RecordIntent]:
    """
    Compares desired vs actual state and returns record intents to add.
    """
    logger.debug("[reconciler] Starting reconciliation additions")

    desired_by_key: Dict[Tuple[str, str, str], RecordIntent] = {_intent_key(r): r for r in desired}
    actual_by_key: Dict[Tuple[str, str, str], RecordIntent] = {_intent_key(r): r for r in actual}

    to_add: List[RecordIntent] = []

    for key, desired_r in desired_by_key.items():
        existing_r = actual_by_key.get(key)

        if not existing_r:
            logger.info(
                f"[reconciler] Adding new record: {desired_r.record.render()} "
                f"(owned by {desired_r.hostname}/{desired_r.container_name})"
            )
            to_add.append(desired_r)
        elif (
            existing_r.hostname == desired_r.hostname
            and existing_r.container_name == desired_r.container_name
        ):
            logger.debug(
                f"[reconciler] Skipping record already owned by us: "
                f"{desired_r.record.render()} (container: {desired_r.container_name})"
            )
        elif desired_r.force:
            logger.info(
                f"[reconciler] Forcibly overriding record owned by {existing_r.hostname}/{existing_r.container_name}: "
                f"{desired_r.record.render()}"
            )
            to_add.append(desired_r)
        elif desired_r.created < existing_r.created:
            # Only override newer record if it's no longer desired
            if _intent_key(existing_r) not in desired_by_key:
                logger.info(
                    f"[reconciler] Overriding stale newer record owned by {existing_r.hostname}/{existing_r.container_name} "
                    f"with older desired record: {desired_r.record.render()}"
                )
                to_add.append(desired_r)
            else:
                logger.debug(
                    f"[reconciler] Existing record is newer and still desired: "
                    f"{existing_r.record.render()}"
                )
        else:
            logger.warning(
                f"[reconciler] Record conflict (not overriding): "
                f"{desired_r.record.render()} already owned by {existing_r.hostname}/{existing_r.container_name}"
            )

    return to_add


def reconcile_removals(
    desired: Iterable[RecordIntent], actual: Iterable[RecordIntent], to_add: Iterable[RecordIntent]
) -> List[RecordIntent]:
    logger.debug("[reconciler] Starting reconciliation removals")

    def record_key(r: RecordIntent) -> Tuple[str, str, str]:
        return (r.record.name, r.record.record_type, str(r.record.value))

    desired_keys = {record_key(r) for r in desired}
    add_keys = {record_key(r) for r in to_add}

    # Only remove records if:
    # - they are owned by us
    # - and not present in either the desired or to_add list
    to_remove = [
        r
        for r in actual
        if r.hostname == settings.hostname and record_key(r) not in desired_keys | add_keys
    ]

    for r in to_remove:
        logger.info(
            f"[reconciler] Removing stale record no longer owned by this host: {r.record.render()}"
        )

    return to_remove
