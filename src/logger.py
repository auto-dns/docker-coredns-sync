import logging
from src.config import load_settings


def setup_logger() -> logging.Logger:
    settings = load_settings()

    logging.basicConfig(
        level=settings.log_level.upper(),
        format="%(asctime)s [%(levelname)s] %(message)s",
    )

    return logging.getLogger("docker_coredns_sync")

logger = setup_logger()
