# src/core/docker_watcher.py
import docker
import threading
import time
from docker.models.containers import Container
from typing import Callable
from logger import logger


class DockerWatcher:
    def __init__(self):
        self.client = docker.from_env()
        self.running = True

    def subscribe(self, callback: Callable[[Container], None]):
        """
        Subscribe to Docker events and invoke the callback with the container object.
        This runs in a background thread.
        """
        thread = threading.Thread(target=self._watch_events, args=(callback,), daemon=True)
        thread.start()

        # Emit all currently running containers at startup
        try:
            for container in self.client.containers.list(filters={"status": "running"}):
                callback(container)
        except Exception as e:
            logger.warning(f"[docker_watcher] Failed to list running containers: {e}")

    def _watch_events(self, callback: Callable[[Container], None]):
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

                try:
                    container = self.client.containers.get(container_id)
                    container.status = status  # manually attach status
                    callback(container)
                except Exception as e:
                    logger.debug(f"[docker_watcher] Could not inspect container {container_id}: {e}")

        except Exception as e:
            logger.error(f"[docker_watcher] Docker event loop error: {e}")
            time.sleep(5)