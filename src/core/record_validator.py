from typing import Iterable

from core.dns_record import ARecord, CNAMERecord
from core.record_intent import RecordIntent
from logger import logger
from utils.errors import RecordValidationError


def validate_record(new_record_intent: RecordIntent, existing_record_intents: Iterable[RecordIntent]) -> None:
	"""
	Validates a proposed DNS record against the current known records.

	Rules enforced:
	1. A and CNAME records may not coexist for the same name.
	2. No duplicate CNAMEs.
	3. A records with the same IP are disallowed for the same name.
	4. CNAMEs may not form resolution cycles.
	"""
	new_record = new_record_intent.record
	existing_records = [r.record for r in existing_record_intents]

	same_name_records = [r for r in existing_records if r.name == new_record.name]
	a_records = [r for r in same_name_records if isinstance(r, ARecord)]
	cname_records = [r for r in same_name_records if isinstance(r, CNAMERecord)]

	# Rule 1: A and CNAME with same name not allowed
	if isinstance(new_record, ARecord):
		if cname_records:
			raise RecordValidationError(f"{new_record.name} -> {new_record.value} - cannot add an A record when a CNAME record exists with the same name")
	elif isinstance(new_record, CNAMERecord):
		if a_records:
			raise RecordValidationError(f"{new_record.name} -> {new_record.value} - cannot add a CNAME record when an A record exists with the same name")
	else:
		raise RecordValidationError(f"Unsupported record type: {type(new_record)}")

	# Rule 2: Multiple CNAMEs with same name not allowed
	if isinstance(new_record, CNAMERecord):
		if cname_records:
			raise RecordValidationError(f"{new_record.name} -> {new_record.value} - cannot have multiple CNAME records with the same name")

	# Rule 3: Multiple A records with the same IP address not allowed
	if isinstance(new_record, ARecord):
		same_ip_records = [r for r in a_records if r.value == new_record.value]
		if same_ip_records:
			raise RecordValidationError(f"{new_record.name} -> {new_record.value} - existing A record(s) detected with the same name and value")

	# Rule 4: Detect cycles
	if isinstance(new_record, CNAMERecord):
		# Construct forwarding map
		forward_map = {}
		for r in existing_records:
			if isinstance(r, CNAMERecord):
				if r.name in forward_map:
					logger.warning(f"Duplicate CNAME definitions detected in remote registry for domain {r.name}")
					continue
			forward_map[r.name] = r.value
		# Add new record to forwarding map
		forward_map[new_record.name] = new_record.value
		# Process to detect loops
		seen = set()
		node = new_record.name
		while node in forward_map:
			if node in seen:
				raise RecordValidationError(f"CNAME cycle detected starting at: {new_record.name}")
			seen.add(node)
			node = forward_map[node]
