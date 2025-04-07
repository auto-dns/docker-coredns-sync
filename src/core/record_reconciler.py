from typing import Iterable, List, Tuple, Dict
from core.record_intent import RecordIntent
from config import load_settings
from logger import logger

settings = load_settings()


def _intent_key(intent: RecordIntent) -> Tuple[str, str, str]:
	"""Create a deduplication key based on record name, type, and value."""
	return (intent.record.name, intent.record.record_type, str(intent.record.value))


def reconcile_additions(desired: Iterable[RecordIntent], actual: Iterable[RecordIntent]) -> List[RecordIntent]:
	"""
	Compares desired vs actual state and returns record intents to add.
	"""
	logger.debug("[reconciler] Starting reconciliation additions")

	desired_by_key: Dict[Tuple[str, str, str], RecordIntent] = {
		_intent_key(r): r for r in desired
	}
	actual_by_key: Dict[Tuple[str, str, str], RecordIntent] = {
		_intent_key(r): r for r in actual
	}

	to_add: List[RecordIntent] = []

	# Detect records to add or override
	for key, desired_r in desired_by_key.items():
		existing_r = actual_by_key.get(key)

		if not existing_r:
			logger.info(f"[reconciler] Adding new record: {desired_r.record.render()} (owned by {desired_r.hostname}/{desired_r.container_name})")
			to_add.append(desired_r)
		else:
			if existing_r.hostname == desired_r.hostname and existing_r.container_name == desired_r.container_name:
				logger.debug(
					f"[reconciler] Skipping record already owned by us: "
					f"{desired_r.record.render()} (container: {desired_r.container_name})"
				)
				continue
			elif desired_r.force:
				logger.info(
					f"[reconciler] Forcibly overriding record owned by {existing_r.hostname}/{existing_r.container_name}: "
					f"{desired_r.record.render()}"
				)
				to_add.append(desired_r)
			elif desired_r.created < existing_r.created:
				logger.info(
					f"[reconciler] Overriding newer record owned by {existing_r.hostname}/{existing_r.container_name} "
					f"with older desired record: {desired_r.record.render()}"
				)
				to_add.append(desired_r)
			else:
				logger.warning(
					f"[reconciler] Record conflict (not overriding): "
					f"{desired_r.record.render()} already owned by {existing_r.hostname}/{existing_r.container_name}"
				)
				continue
	
	return to_add


def reconcile_removals(actual: Iterable[RecordIntent], raw_desired: Iterable[RecordIntent]) -> List[RecordIntent]:
    logger.debug("[reconciler] Starting reconciliation removals")

    def record_key(r: RecordIntent) -> Tuple[str, str, str]:
        return (r.record.name, r.record.record_type, str(r.record.value))

    # All current raw desired keys (from this host's active containers)
    raw_desired_keys = {record_key(r) for r in raw_desired if r.hostname == settings.hostname}

    # Actual keys currently in etcd owned by this host
    actual_keys = {record_key(r): r for r in actual if r.hostname == settings.hostname}

    # Only delete actual records if:
    # - they are owned by this host
    # - and they are NOT claimed by any currently-running container
    to_remove = [r for key, r in actual_keys.items() if key not in raw_desired_keys]

    for r in to_remove:
        logger.info(f"[reconciler] Removing stale record no longer owned by this host: {r.record.render()}")

    return to_remove
