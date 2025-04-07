import etcd3
import json
import time
from typing import List, Union
from contextlib import contextmanager
from datetime import datetime

from config import load_settings
from core.dns_record import Record, ARecord, CNAMERecord
from core.record_intent import RecordIntent
from interfaces.registry_with_lock import RegistryWithLock
from logger import logger
from utils.errors import (
	EtcdConnectionError,
	RegistryUnsupportedRecordTypeError,
	RegistryParseError
)

settings = load_settings()


class EtcdRegistry(RegistryWithLock):
	def __init__(self):
		try:
			self.client = etcd3.client(host=settings.etcd_host, port=settings.etcd_port)
		except Exception as e:
			raise EtcdConnectionError(f"Failed to connect to etcd: {e}")

	def _get_next_indexed_key(self, fqdn: str) -> str:
		parts = fqdn.strip(".").split(".")[::-1]
		base_key = f"{settings.etcd_path_prefix}/{'/'.join(parts)}"
		existing_indices = set()
		for _, meta in self.client.get_prefix(base_key):
			key = meta.key.decode()
			suffix = key.rsplit("/", 1)[-1]
			if suffix.startswith("x"):
				try:
					existing_indices.add(int(suffix[1:]))
				except ValueError:
					continue
		index = 1
		while index in existing_indices:
			index += 1
		return f"{base_key}/x{index}"

	def register(self, record_intent: RecordIntent) -> None:
		fqdn = record_intent.record.name
		key = self._get_next_indexed_key(fqdn)
		value = self._get_etcd_value(record_intent)
		self.client.put(key, value)

	def remove(self, record_intent: RecordIntent) -> None:
		fqdn = record_intent.record.name
		parts = fqdn.strip(".").split(".")[::-1]
		base_key = f"{settings.etcd_path_prefix}/{'/'.join(parts)}"

		for value, meta in self.client.get_prefix(base_key):
			key = meta.key.decode()
			try:
				existing = json.loads(value)
				if (
					existing.get("host") == str(record_intent.record.value)
					and existing.get("record_type") == record_intent.record.record_type
					and existing.get("owner_hostname") == record_intent.hostname
					and existing.get("owner_container_name") == record_intent.container_name
				):
					self.client.delete(key)
					logger.info(f"[etcd_registry] Deleted key {key}")
					return
			except Exception as e:
				logger.warning(f"[etcd_registry] Could not parse or match key {key}: {e}")

	def list(self) -> List[RecordIntent]:
		prefix = settings.etcd_path_prefix
		record_intents = []
		for value, meta in self.client.get_prefix(prefix):
			try:
				record_intent = self._parse_etcd_value(meta.key.decode(), value.decode())
				record_intents.append(record_intent)
			except Exception as e:
				logger.error(f"[etcd_registry] Failed to parse key: {meta.key}: {e}")
		return record_intents

	@contextmanager
	def lock_transaction(self, keys: Union[str, List[str]]):
		if isinstance(keys, str):
			keys = [keys]  # Backward-compatible

		keys = sorted(set(keys))  # Ensure consistent ordering to avoid deadlocks
		leases = []

		try:
			for key in keys:
				lock_key = f"/locks/{key}"
				lease = self.client.lease(settings.etcd_lock_ttl)
				acquired = False
				start = time.time()

				while time.time() - start < settings.etcd_lock_timeout:
					success, _ = self.client.transaction(
						compare=[self.client.transactions.create(lock_key) == 0],
						success=[self.client.transactions.put(lock_key, settings.hostname, lease)],
						failure=[],
					)
					if success:
						acquired = True
						leases.append((lock_key, lease))
						break

					time.sleep(settings.etcd_lock_retry_interval)

				if not acquired:
					raise EtcdConnectionError(f"Failed to acquire lock on {key}")

			yield

		finally:
			for lock_key, lease in reversed(leases):
				try:
					self.client.delete(lock_key)
					lease.revoke()
				except Exception:
					pass

	def _get_etcd_key(self, record_intent: RecordIntent) -> str:
		# Format: ***REMOVED***/api
		parts = record_intent.record.name.strip(".").split(".")[::-1]
		return f"{settings.etcd_path_prefix}/{'/'.join(parts)}"

	def _get_etcd_value(self, record_intent: RecordIntent) -> str:
		if isinstance(record_intent.record, (ARecord, CNAMERecord)):
			return json.dumps({
				"host": str(record_intent.record.value),
				"record_type": record_intent.record.record_type,
				"owner_hostname": record_intent.hostname,
				"owner_container_name": record_intent.container_name,
				"created": record_intent.created.isoformat(),
			})
		else:
			logger.warning(f"[etcd_registry] Unsupported record object: {record_intent.record}")
			raise RegistryUnsupportedRecordTypeError(f"Unsupported record type: {record_intent.record.record_type}")

	def _parse_etcd_value(self, key: str, value: str) -> RecordIntent:
		# TODO: Remove these if we run the code and don't get circular import errors
		# from core.dns_record import ARecord, CNAMERecord  # avoid circular import
		# from utils.errors import RegistryParseError

		path = key[len(settings.etcd_path_prefix):].lstrip("/")
		labels = path.split("/")[::-1]
		name = ".".join(labels)

		data = json.loads(value)
		record_type = data.get("record_type")
		host = data.get("host")
		owner_hostname = data.get("owner_hostname")
		owner_container_name = data.get("owner_container_name")
		created_str = data.get("created")
		created = datetime.fromisoformat(created_str.replace("Z", "+00:00"))

		if not record_type or not host:
			raise RegistryParseError(f"Missing required fields in etcd record: {data}")

		if record_type.upper() == "A":
			return RecordIntent(
				container_id="<from-etcd>",
				container_name=owner_container_name,
				created=created,
				hostname=owner_hostname,
				record=ARecord(
					name=name,
					value=host,
				),
			)
		elif record_type.upper() == "CNAME":
			return RecordIntent(
				container_id="<from-etcd>",
				container_name=owner_container_name,
				created=created,
				hostname=owner_hostname,
				record=CNAMERecord(
					name=name,
					value=host,
				)
			)

		raise RegistryUnsupportedRecordTypeError(f"Unknown record type: {record_type}")