import threading
import time
from datetime import datetime, timezone
from typing import Callable

import docker
from docker.models.containers import Container

from src.core.container_event import ContainerEvent
from src.logger import logger
from src.utils.timing import retry


class DockerWatcher:
    def __init__(self) -> None:
        logger.debug("[docker_watcher] Initializing Docker watcher")
        self.client = docker.from_env()
        self.running = True

    def subscribe(self, callback: Callable[[ContainerEvent], None]) -> None:
        """
        Subscribe to Docker events and invoke the callback with a ContainerEvent.
        This runs in a background thread.
        """
        logger.info("[docker_watcher] Starting Docker event subscription thread")
        thread = threading.Thread(target=self._watch_events, args=(callback,), daemon=True)
        thread.start()

        # Emit all currently running containers at startup
        try:
            logger.info("[docker_watcher] Listing currently running containers")
            for container in self.client.containers.list(filters={"status": "running"}):
                logger.debug(f"[docker_watcher] Found running container: {container.id}")
                callback(self._build_container_event(container, status="start"))
        except Exception as e:
            logger.warning(f"[docker_watcher] Failed to list running containers: {e}")

    def _watch_events(self, callback: Callable[[ContainerEvent], None]) -> None:
        """
        Internal loop that listens to Docker events and calls the callback.
        """
        logger.info("[docker_watcher] Watching for Docker events...")
        try:
            for event in self.client.events(decode=True):  # type: ignore[no-untyped-call]
                if not self.running:
                    logger.info("[docker_watcher] Stopping event watch loop")
                    break

                if event.get("Type") != "container":
                    continue

                status = event.get("status")
                container_id = event.get("id")

                if status not in {"start", "die", "stop", "destroy"}:
                    continue

                logger.debug(f"[docker_watcher] Received container event: {status} for container {container_id}")

                if status == "start":
                    try:
                        logger.debug(f"[docker_watcher] Getting container info for {container_id}")
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
        logger.debug(f"[docker_watcher] Attempting to get container {container_id}")
        return self.client.containers.get(container_id)

    def _build_container_event(self, container: Container, status: str) -> ContainerEvent:
        try:
            created_str = container.attrs.get("Created", "")
            created = datetime.fromisoformat(created_str.replace("Z", "+00:00")).astimezone(
                timezone.utc
            )
        except Exception:
            logger.debug(f"[docker_watcher] Failed to parse creation time for container {container.id}")
            created = None

        logger.debug(f"[docker_watcher] Building container event for {container.id} with status {status}")
        return ContainerEvent(
            id=container.id,
            name=getattr(container, "name", "<unknown>"),
            created=created,
            status=status,
            labels=getattr(container, "labels", {}),
            attrs=getattr(container, "attrs", {}),
        )

    def stop(self) -> None:
        logger.info("[docker_watcher] Stopping Docker watcher")
        self.running = False