import etcd3
import json
import time
from typing import List
from contextlib import contextmanager

from config import load_settings
from core.dns_record import Record, ARecord, CNAMERecord
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

	def register(self, record: Record) -> None:
		key = self._get_etcd_key(record)
		value = self._get_etcd_value(record)
		self.client.put(key, value)

	def remove(self, record: Record) -> None:
		key = self._get_etcd_key(record)
		self.client.delete(key)

	def list(self) -> List[Record]:
		prefix = settings.etcd_path_prefix
		records = []
		for value, meta in self.client.get_prefix(prefix):
			try:
				record = self._parse_etcd_value(meta.key.decode(), value.decode())
				records.append(record)
			except Exception as e:
				logger.error(f"[etcd_registry] Failed to parse key: {meta.key}: {e}")
		return records

	@contextmanager
	def lock_transaction(self, keys: str | list[str]):
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

	def _get_etcd_key(self, record: Record) -> str:
		# Format: ***REMOVED***/api
		parts = record.name.strip(".").split(".")[::-1]
		return f"{settings.etcd_path_prefix}/{'/'.join(parts)}"

	def _get_etcd_value(self, record: Record) -> str:
		if isinstance(record, (ARecord, CNAMERecord)):
			return json.dumps({
				"host": str(record.value),
				"record_type": record.record_type,
				"owner": record.owner,
			})
		raise RegistryUnsupportedRecordTypeError(f"Unsupported record type: {record.record_type}")

	def _parse_etcd_value(self, key: str, value: str) -> Record:
		# TODO: Remove these if we run the code and don't get circular import errors
		# from core.dns_record import ARecord, CNAMERecord  # avoid circular import
		# from utils.errors import RegistryParseError

		path = key[len(settings.etcd_path_prefix):].lstrip("/")
		labels = path.split("/")[::-1]
		name = ".".join(labels)

		data = json.loads(value)
		record_type = data.get("record_type")
		host = data.get("host")
		owner = data.get("owner")

		if not record_type or not host or not owner:
			raise RegistryParseError(f"Missing required fields in etcd record: {data}")

		if record_type.upper() == "A":
			return ARecord(name=name, value=host, owner=owner)
		elif record_type.upper() == "CNAME":
			return CNAMERecord(name=name, value=host, owner=owner)

		raise RegistryUnsupportedRecordTypeError(f"Unknown record type: {record_type}")