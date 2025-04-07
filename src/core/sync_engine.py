import time
from core.container_event import ContainerEvent
from core.docker_watcher import DockerWatcher
from core.record_builder import get_container_record_intents
from core.record_reconciler import reconcile_records
from core.record_validator import validate_record
from core.state import StateTracker
from datetime import datetime
from interfaces.registry_interface import DnsRegistry
from logger import logger


class SyncEngine:
    def __init__(self, registry: DnsRegistry, poll_interval: float = 5.0):
        self.registry = registry
        self.poll_interval = poll_interval
        self.state = StateTracker()
        self.watcher = DockerWatcher()
        self.running = False

    def handle_event(self, event: ContainerEvent):
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

    def run(self):
        self.running = True
        self.watcher.subscribe(self.handle_event)

        while self.running:
            try:
                # Fetch the current state (local docker container record_intents, remote etcd record_intents)
                actual_record_intents = self.registry.list()
                desired_record_intents = self.state.get_all_desired_record_intents()

                # Step 1: Reconcile — compute records to add/remove
                to_add, to_remove = reconcile_records(desired_record_intents, actual_record_intents)

                # Step 2: Validate adds individually
                valid_adds = []
                for record_intent in to_add:
                    try:
                        validate_record(record_intent, actual_record_intents + valid_adds)
                        valid_adds.append(record_intent)
                    except Exception as e:
                        # TODO: should I create a render function for the record intent and just call that? Or does it already have a render function from pydantic?
                        logger.warning(f"[validator] Skipping invalid record {record_intent.record.render()} — {e}")

                # Step 3: Apply — remove first, then add
                for r in to_remove:
                    self.registry.remove(r)
                for r in valid_adds:
                    self.registry.register(r)

                # Step 4: Expire stale containers
                self.state.remove_stale(ttl=60)

            except Exception as e:
                logger.error(f"[sync_engine] Sync error: {e}")

            time.sleep(self.poll_interval)

    def stop(self):
        self.running = False
        self.watcher.stop()
        logger.info("[sync_engine] Graceful shutdown initiated.")
