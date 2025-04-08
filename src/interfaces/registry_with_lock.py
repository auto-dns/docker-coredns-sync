# src/interfaces/registry_with_lock.py
from contextlib import AbstractContextManager
from typing import Protocol

from interfaces.registry_interface import DnsRegistry


class RegistryWithLock(DnsRegistry, Protocol):
    def lock_transaction(self, key: str) -> AbstractContextManager:
        """
        Context manager for safely coordinating a list/validate/register transaction.

        `key` is typically a record name (FQDN) or name group to scope the lock.

        Example usage:
        with registry.lock_transaction("***REMOVED***"):
            ...
        """
        ...
