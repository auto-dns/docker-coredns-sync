from core.dns_record import Record, ARecord, CNAMERecord
from utils.errors import RecordValidationError
from typing import Iterable


def validate_record(new_record: Record, existing_records: Iterable[Record]) -> None:
	"""
	Validate a single record against a full list of existing records.

	Raises:
		RecordValidationError if any rule is violated.
	"""
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
		forward_map = {r.name: r.value for r in existing_records if isinstance(r, CNAMERecord)}
		forward_map[new_record.name] = new_record.value
		seen = set()
		node = new_record.name
		while node in forward_map:
			if node in seen:
				raise RecordValidationError(f"CNAME cycle detected starting at: {new_record.name}")
			seen.add(node)
			node = forward_map[node]
