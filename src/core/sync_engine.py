import time
from core.state import StateTracker
from core.record_validator import validate_record
from core.record_reconciler import reconcile_records
from interfaces.registry_interface import DnsRegistry
from core.docker_watcher import DockerWatcher
from core.record_builder import build_records_from_container
from logger import logger


class SyncEngine:
    def __init__(self, registry: DnsRegistry, poll_interval: float = 5.0):
        self.registry = registry
        self.poll_interval = poll_interval
        self.state = StateTracker()
        self.watcher = DockerWatcher()

    def handle_event(self, container):
        if not container:
            return

        if container.status == "running":
            records = build_records_from_container(container)
            if records:
                self.state.upsert(container.id, records, "running")
        else:
            self.state.mark_removed(container.id)

    def run(self):
        self.watcher.subscribe(self.handle_event)

        while True:
            try:
                # Fetch the current state (local docker container records, remote etcd records)
                actual_records = self.registry.list()
                desired_records = self.state.get_all_desired_records()

                # Step 1: Reconcile — compute records to add/remove
                to_add, to_remove = reconcile_records(desired_records, actual_records)

                # Step 2: Validate adds individually
                valid_adds = []
                for record in to_add:
                    try:
                        validate_record(record, actual_records + valid_adds)
                        valid_adds.append(record)
                    except Exception as e:
                        logger.warning(f"[validator] Skipping invalid record {record.render()} — {e}")

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