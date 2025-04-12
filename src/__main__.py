import signal
import sys
from typing import Any

from src.backends.etcd_registry import EtcdRegistry
from src.core.sync_engine import SyncEngine


def main() -> None:
    registry = EtcdRegistry()
    engine = SyncEngine(registry)

    def shutdown_handler(signum: int, frame: Any) -> None:
        print("Shutting down...")
        engine.stop()
        sys.exit(0)

    # Register signal handlers
    signal.signal(signal.SIGINT, shutdown_handler)
    signal.signal(signal.SIGTERM, shutdown_handler)

    engine.run()


if __name__ == "__main__":
    main()
