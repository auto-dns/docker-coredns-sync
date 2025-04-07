from typing import Iterable, List, Tuple, Dict
from core.record_intent import RecordIntent
from config import load_settings
from logger import logger

settings = load_settings()


def _intent_key(intent: RecordIntent) -> Tuple[str, str, str]:
	"""Create a deduplication key based on record name, type, and value."""
	return (intent.record.name, intent.record.record_type, str(intent.record.value))


def reconcile_records(
	desired: Iterable[RecordIntent],
	actual: Iterable[RecordIntent]
) -> Tuple[List[RecordIntent], List[RecordIntent]]:
	"""
	Compares desired vs actual state and returns (to_add, to_remove) record intents.
	"""
	logger.debug("[reconciler] Starting reconciliation process")

	desired_by_key: Dict[Tuple[str, str, str], RecordIntent] = {
		_intent_key(r): r for r in desired
	}
	actual_by_key: Dict[Tuple[str, str, str], RecordIntent] = {
		_intent_key(r): r for r in actual
	}

	to_add: List[RecordIntent] = []
	to_remove: List[RecordIntent] = []

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
				to_remove.append(existing_r)
				to_add.append(desired_r)
			elif desired_r.created < existing_r.created:
				logger.info(
					f"[reconciler] Overriding newer record owned by {existing_r.hostname}/{existing_r.container_name} "
					f"with older desired record: {desired_r.record.render()}"
				)
				to_remove.append(existing_r)
				to_add.append(desired_r)
			else:
				logger.warning(
					f"[reconciler] Record conflict (not overriding): "
					f"{desired_r.record.render()} already owned by {existing_r.hostname}/{existing_r.container_name}"
				)
				continue

	# Remove stale records that this host previously registered
	owned_keys = {
		_intent_key(r) for r in desired
		if r.hostname == settings.hostname
	}

	for key, actual_r in actual_by_key.items():
		if actual_r.hostname == settings.hostname and key not in owned_keys:
			logger.info(f"[reconciler] Removing stale record no longer owned by this host: {actual_r.record.render()}")
			to_remove.append(actual_r)

	logger.debug(f"[reconciler] Reconciliation complete. {len(to_add)} to add, {len(to_remove)} to remove.")
	return to_add, to_remove
