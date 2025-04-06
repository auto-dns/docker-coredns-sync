import logging
from config import load_settings


logging.getLogger("urllib3").setLevel(logging.WARNING)
logging.getLogger("docker").setLevel(logging.WARNING)

def setup_logger() -> logging.Logger:
    settings = load_settings()

    logging.basicConfig(
        level=settings.log_level.upper(),
        format="%(asctime)s [%(levelname)s] %(message)s",
    )

    return logging.getLogger("docker_coredns_sync")

logger = setup_logger()
