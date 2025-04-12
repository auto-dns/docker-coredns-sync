import time

from src.config import load_settings
from src.core.container_event import ContainerEvent
from src.core.docker_watcher import DockerWatcher
from src.core.record_builder import get_container_record_intents
from src.core.record_reconciler import filter_record_intents, reconcile_and_validate
from src.core.state import StateTracker
from src.interfaces.registry_with_lock import RegistryWithLock
from src.logger import logger

settings = load_settings()


class SyncEngine:
    def __init__(self, registry: RegistryWithLock, poll_interval: float = 5.0):
        self.registry = registry
        self.poll_interval = poll_interval
        self.state = StateTracker()
        self.watcher = DockerWatcher()
        self.running = False

    def handle_event(self, event: ContainerEvent) -> None:
        if not event:
            return

        if event.status == "start":
            record_intents = get_container_record_intents(event)
            if record_intents:
                self.state.upsert(
                    container_id=event.id,
                    container_name=event.name,
                    container_created=event.created,
                    record_intents=record_intents,
                    status="running",
                )
        else:
            self.state.mark_removed(event.id)

    def run(self) -> None:
        self.running = True
        self.watcher.subscribe(self.handle_event)

        while self.running:
            try:
                with self.registry.lock_transaction("__global__"):
                    actual_record_intents = self.registry.list()
                    desired_record_intents = self.state.get_all_desired_record_intents()
                    desired_record_intents_reconciled = filter_record_intents(desired_record_intents)

                    to_add, to_remove = reconcile_and_validate(desired_record_intents_reconciled, actual_record_intents)

                    for r in to_remove:
                        self.registry.remove(r)
                    for r in to_add:
                        self.registry.register(r)

            except Exception as e:
                logger.exception(f"[sync_engine] Sync error: {e}")

            time.sleep(self.poll_interval)

    def stop(self) -> None:
        self.running = False
        self.watcher.stop()
        logger.info("[sync_engine] Graceful shutdown initiated.")
