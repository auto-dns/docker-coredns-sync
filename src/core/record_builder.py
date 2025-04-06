from config import load_settings
from core.dns_record import ARecord, CNAMERecord
from core.record_intent import RecordIntent
from datetime import datetime, timezone
from logger import logger

settings = load_settings()

def _get_force(labels, container_force_label, record_force_label):
	container_force = labels.get(container_force_label, "false").lower() == "true"
	record_force_label_exists = record_force_label in labels
	record_force = labels.get(record_force_label, "false").lower() == "true"
	force = record_force if record_force_label_exists else container_force
	return force


def get_container_record_intents(container) -> list[RecordIntent]:
	labels = container.labels
	allowed_record_types = set(settings.allowed_record_types)
	prefix = settings.docker_label_prefix

	if labels.get(f"{prefix}.enabled", "false").lower() != "true":
		return []

    # Collapse labels as such:

	# coredns.A.name=source
	# coredns.A.value=target
	# ->
	# {"name": "source": "value": "target"}
	base_label_pairs = {}

	# coredns.A.alias1.name=source
	# coredns.A.alias1.value=target
	# ->
	# {
	# 	"alias1": {
	# 		"name": "source",
	# 		"value": "target"
	#	}
	# }
	aliased_label_pairs = {}

	for label, value in labels.items():
		parts = label.split(".")
		type = parts[1]

		if type not in allowed_record_types:
			logger.error(f"[record_builder] Unsupported record type {type}")
			continue

		if len(parts) == 3 and parts[0] == prefix and parts[2] in {"name", "value"}:
			# Format 1 (base): prefix.type.key(name|value)
			key = parts[2]
			
			if type not in base_label_pairs:
				base_label_pairs[type] = {}
			
			base_label_pairs[type][key] = value
		elif len(parts) >= 4 and parts[0] == prefix and parts[3] in {"name", "value"}:
			# Format 2 (aliased): prefix.type.alias.key(name|value)
			alias = parts[2]
			key = parts[3]
			
			if type not in aliased_label_pairs:
				aliased_label_pairs[type] = {}

			if alias not in aliased_label_pairs[type]:
				aliased_label_pairs[type][alias] = {}
			
			aliased_label_pairs[type][alias][key] = value

	record_intents: list[RecordIntent] = []
	container_id = container.id
	container_name = getattr(container, "name", "<unknown>")
	container_created_str = container.attrs["Created"]
	container_created = datetime.fromisoformat(container_created_str.replace("Z", "+00:00")).astimezone(timezone.utc)
	hostname = settings.hostname
	container_force_label = f"{prefix}.force"
	
	# Pass over each set, filtering out invalid records and using default values ones with missing values
	# Handle base label pairs first
	if "A" in base_label_pairs:
		if "name" in base_label_pairs["A"]:
			name = base_label_pairs["A"]["name"]
			if "value" in base_label_pairs["A"]:
				value = base_label_pairs["A"]["value"]
			else:
				value = settings.host_ip
				logger.warning(f"[record_builder] {prefix}.A.name={base_label_pairs['A']['name']} label found with no matching {prefix}.A.value pair. Using configured host IP {value} as default.")
			try:
				force = _get_force(labels=labels, container_force_label=container_force_label, record_force_label=f"{prefix}.A.force")
				record_intents.append(
					RecordIntent(
						container_id=container_id,
						container_name=container_name,
						created=container_created,
						force=force,
						hostname=hostname,
						record=ARecord(
							name=name,
							value=value,
						),
					)
				)
			except ValueError as e:
				logger.warning(f"[record_builder] Invalid ARecord {name}: {e}")
		elif "value" in base_label_pairs["A"]:
			logger.error(f"[record_builder] {prefix}.A.value={base_label_pairs['A']['value']} label found with no matching {prefix}.A.name pair.")
	if "CNAME" in base_label_pairs:
		if "name" in base_label_pairs["CNAME"] and "value" in base_label_pairs["CNAME"]:
			name = base_label_pairs["CNAME"]["name"]
			value = base_label_pairs["CNAME"]["value"]
			try:
				force = _get_force(labels=labels, container_force_label=container_force_label, record_force_label=f"{prefix}.CNAME.force")
				record_intents.append(
					RecordIntent(
						container_id=container_id,
						container_name=container_name,
						created=container_created,
						force=force,
						hostname=hostname,
						record=CNAMERecord(
							name=name,
							value=value,
						),
					)
				)
			except ValueError as e:
				logger.warning(f"[record_builder] Invalid CNAMERecord {name}: {e}")
		elif "name" in base_label_pairs["CNAME"] and "value" not in base_label_pairs["CNAME"]:
			logger.error(f"[record_builder] {prefix}.CNAME.name={base_label_pairs['CNAME']['name']} label found with no matching {prefix}.CNAME.value pair.")
		elif "value" in base_label_pairs["CNAME"] and "name" not in base_label_pairs["CNAME"]:
			logger.error(f"[record_builder] {prefix}.CNAME.value={base_label_pairs['CNAME']['value']} label found with no matching {prefix}.CNAME.name pair.")

	# Handle aliased label pairs next
	if "A" in aliased_label_pairs:
		for alias, pair in aliased_label_pairs["A"]:
			if "name" in pair:
				name = pair["name"]
				if "value" in pair:
					value = pair["value"]
				else:
					value = settings.host_ip
					logger.warning(f"[record_builder] {prefix}.A.name={pair['name']} label found with no matching {prefix}.A.value pair. Using configured host IP {value} as default.")
				try:
					force = _get_force(labels=labels, container_force_label=container_force_label, record_force_label=f"{prefix}.A.{alias}.force")
					record_intents.append(
						RecordIntent(
							container_id=container_id,
							container_name=container_name,
							created=container_created,
							force=force,
							hostname=hostname,
							record=ARecord(
								name=name,
								value=value,
							),
						)
					)
				except ValueError as e:
					logger.warning(f"[record_builder] Invalid ARecord {name}: {e}")
			elif "value" in pair:
				logger.error(f"[record_builder] {prefix}.A.value={pair['value']} label found with no matching {prefix}.A.name pair.")
	if "CNAME" in aliased_label_pairs:
		for alias, pair in aliased_label_pairs["CNAME"]:
			if "name" in pair and "value" in pair:
				name = base_label_pairs["CNAME"]["name"]
				value = base_label_pairs["CNAME"]["value"]
				try:
					force = _get_force(labels=labels, container_force_label=container_force_label, record_force_label=f"{prefix}.CNAME.{alias}.force")
					record_intents.append(
						RecordIntent(
							container_id=container_id,
							container_name=container_name,
							created=container_created,
							force=force,
							hostname=hostname,
							record=CNAMERecord(
								name=name,
								value=value,
							),
						)
					)
				except ValueError as e:
					logger.warning(f"[record_builder] Invalid CNAMERecord {name}: {e}")
			elif "name" in pair and "value" not in pair:
				logger.error(f"[record_builder] {prefix}.CNAME.name={pair['name']} label found with no matching {prefix}.CNAME.value pair.")
			elif "value" in pair and "name" not in pair:
				logger.error(f"[record_builder] {prefix}.CNAME.value={pair['value']} label found with no matching {prefix}.CNAME.name pair.")

	return record_intents