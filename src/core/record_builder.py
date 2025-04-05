from config import load_settings
from core.dns_record import ARecord, CNAMERecord, Record
from logger import logger

settings = load_settings()


def get_container_records(container) -> list[Record]:
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

	for label, value in labels:
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

	records: list[Record] = []
	
	# Pass over each set, filtering out invalid records and using default values ones with missing values
	# Handle base label pairs first
	if "A" in base_label_pairs:
		if "name" in base_label_pairs["A"]:
			name = base_label_pairs["A"]["name"]
			if "value" in base_label_pairs["A"]:
				value = base_label_pairs["A"]["value"]
			else:
				value = settings.host_ip
				logger.warning(f"[record_builder] {prefix}.A.name={base_label_pairs["A"]["name"]} label found with no matching {prefix}.A.value pair. Using configured host IP {value} as default.")
			try:
				records.append(ARecord(name=name, value=value, owner=settings.hostname))
			except ValueError as e:
				logger.warning(f"[record_builder] Invalid ARecord {name}: {e}")
		elif "value" in base_label_pairs["A"]:
			logger.error(f"[record_builder] {prefix}.A.value={base_label_pairs["A"]["value"]} label found with no matching {prefix}.A.name pair.")
	if "CNAME" in base_label_pairs:
		if "name" in base_label_pairs["CNAME"] and "value" in base_label_pairs["CNAME"]:
			name = base_label_pairs["CNAME"]["name"]
			value = base_label_pairs["CNAME"]["value"]
			try:
				records.append(CNAMERecord(name=name, value=value, owner=settings.hostname))
			except ValueError as e:
				logger.warning(f"[record_builder] Invalid CNAMERecord {name}: {e}")
		elif "name" in base_label_pairs["CNAME"] and "value" not in base_label_pairs["CNAME"]:
			logger.error(f"[record_builder] {prefix}.CNAME.name={base_label_pairs["CNAME"]["name"]} label found with no matching {prefix}.CNAME.value pair.")
		elif "value" in base_label_pairs["CNAME"] and "name" not in base_label_pairs["CNAME"]:
			logger.error(f"[record_builder] {prefix}.CNAME.value={base_label_pairs["CNAME"]["value"]} label found with no matching {prefix}.CNAME.name pair.")

	# Handle aliased label pairs next
	if "A" in aliased_label_pairs:
		for alias, pair in aliased_label_pairs["A"]:
			if "name" in pair:
				name = pair["name"]
				if "value" in pair:
					value = pair["value"]
				else:
					value = settings.host_ip
					logger.warning(f"[record_builder] {prefix}.A.name={pair["name"]} label found with no matching {prefix}.A.value pair. Using configured host IP {value} as default.")
				try:
					records.append(ARecord(name=name, value=value, owner=settings.hostname))
				except ValueError as e:
					logger.warning(f"[record_builder] Invalid ARecord {name}: {e}")
			elif "value" in pair:
				logger.error(f"[record_builder] {prefix}.A.value={pair["value"]} label found with no matching {prefix}.A.name pair.")
	if "CNAME" in aliased_label_pairs:
		for alias, pair in aliased_label_pairs["CNAME"]:
			if "name" in pair and "value" in pair:
				name = base_label_pairs["CNAME"]["name"]
				value = base_label_pairs["CNAME"]["value"]
				try:
					records.append(CNAMERecord(name=name, value=value, owner=settings.hostname))
				except ValueError as e:
					logger.warning(f"[record_builder] Invalid CNAMERecord {name}: {e}")
			elif "name" in pair and "value" not in pair:
				logger.error(f"[record_builder] {prefix}.CNAME.name={pair["name"]} label found with no matching {prefix}.CNAME.value pair.")
			elif "value" in pair and "name" not in pair:
				logger.error(f"[record_builder] {prefix}.CNAME.value={pair["value"]} label found with no matching {prefix}.CNAME.name pair.")

	return records