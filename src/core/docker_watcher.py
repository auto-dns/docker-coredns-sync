# src/core/docker_watcher.py

import threading
import time
from datetime import datetime, timezone
from typing import Callable

import docker
from docker.models.containers import Container

from core.container_event import ContainerEvent
from logger import logger
from utils.timing import retry


class DockerWatcher:
    def __init__(self):
        self.client = docker.from_env()
        self.running = True

    def subscribe(self, callback: Callable[[ContainerEvent], None]):
        """
        Subscribe to Docker events and invoke the callback with a ContainerEvent.
        This runs in a background thread.
        """
        thread = threading.Thread(target=self._watch_events, args=(callback,), daemon=True)
        thread.start()

        # Emit all currently running containers at startup
        try:
            for container in self.client.containers.list(filters={"status": "running"}):
                callback(self._build_container_event(container, status="start"))
        except Exception as e:
            logger.warning(f"[docker_watcher] Failed to list running containers: {e}")

    def _watch_events(self, callback: Callable[[ContainerEvent], None]):
        """
        Internal loop that listens to Docker events and calls the callback.
        """
        logger.info("[docker_watcher] Watching for Docker events...")
        try:
            for event in self.client.events(decode=True):
                if not self.running:
                    break

                if event.get("Type") != "container":
                    continue

                status = event.get("status")
                container_id = event.get("id")

                if status not in {"start", "die", "stop", "destroy"}:
                    continue

                if status == "start":
                    try:
                        container = self._safe_get_container(container_id)
                        callback(self._build_container_event(container, status=status))
                    except Exception as e:
                        logger.warning(
                            f"[docker_watcher] Failed to inspect container after start: {e}"
                        )
                else:
                    logger.debug(
                        f"[docker_watcher] Container {container_id} {status} event — marking as removed."
                    )
                    callback(
                        ContainerEvent(id=container_id, status=status)
                    )  # Minimal data for stop/die

        except Exception as e:
            logger.error(f"[docker_watcher] Docker event loop error: {e}")
            time.sleep(5)

    @retry(retries=3, delay=0.5, logger_func=logger.error)
    def _safe_get_container(self, container_id: str) -> Container:
        return self.client.containers.get(container_id)

    def _build_container_event(self, container: Container, status: str) -> ContainerEvent:
        try:
            created_str = container.attrs.get("Created", "")
            created = datetime.fromisoformat(created_str.replace("Z", "+00:00")).astimezone(
                timezone.utc
            )
        except Exception:
            created = None

        return ContainerEvent(
            id=container.id,
            name=getattr(container, "name", "<unknown>"),
            created=created,
            status=status,
            labels=getattr(container, "labels", {}),
            attrs=getattr(container, "attrs", {}),
        )

    def stop(self):
        self.running = False
