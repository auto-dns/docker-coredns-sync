import signal
import sys

from backends.etcd_registry import EtcdRegistry
from core.sync_engine import SyncEngine


def main():
    registry = EtcdRegistry()
    engine = SyncEngine(registry)

    def shutdown_handler(signum, frame):
        print("Shutting down...")
        engine.stop()
        sys.exit(0)

    # Register signal handlers
    signal.signal(signal.SIGINT, shutdown_handler)
    signal.signal(signal.SIGTERM, shutdown_handler)

    engine.run()


if __name__ == "__main__":
    main()
