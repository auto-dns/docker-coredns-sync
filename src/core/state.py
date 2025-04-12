import time
from datetime import datetime
from typing import Dict, List

from src.core.record_intent import RecordIntent


class ContainerState:
    def __init__(
        self,
        container_id: str,
        container_name: str,
        container_created: datetime,
        record_intents: List[RecordIntent],
        status: str,
    ):
        self.container_id = container_id
        self.container_name = container_name
        self.container_created = container_created
        self.record_intents = record_intents
        self.status = status  # "running", "removed"
        self.last_updated = time.time()

    def is_stale(self, ttl: float = 30.0) -> bool:
        return (time.time() - self.last_updated) > ttl


class StateTracker:
    def __init__(self) -> None:
        self._containers: Dict[str, ContainerState] = {}

    def upsert(
        self,
        container_id: str,
        container_name: str,
        container_created: datetime,
        record_intents: List[RecordIntent],
        status: str,
    ) -> None:
        self._containers[container_id] = ContainerState(
            container_id, container_name, container_created, record_intents, status
        )

    def mark_removed(self, container_id: str) -> None:
        if container_id in self._containers:
            self._containers[container_id].status = "removed"
            self._containers[container_id].last_updated = time.time()

    def get_all_desired_record_intents(self) -> List[RecordIntent]:
        result = []
        for state in self._containers.values():
            if state.status == "running":
                result.extend(state.record_intents)
        return result

    def remove_stale(self, ttl: float = 60.0) -> None:
        # Unused at the moment.
        # Usage will require a recurring docker check to update lists of running containers
        # but this is redundant based on the way we process events
        expired = [cid for cid, state in self._containers.items() if state.is_stale(ttl)]
        for cid in expired:
            del self._containers[cid]
