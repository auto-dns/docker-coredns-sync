import os

HOST_IP = os.getenv("HOST_IP", "127.0.0.1")
REDIS_HOST = os.getenv("REDIS_HOST", "localhost")
REDIS_PORT = os.getenv("REDIS_PORT", 6379)
REDIS_PASSWORD = os.getenv("REDIS_PASSWORD", None)
REDIS_PREFIX = os.getenv("REDIS_PREFIX", "_dns:")
HOSTNAME = os.getenv("HOSTNAME", "your-hostname")
LABEL_PREFIX = os.getenv("LABEL_PREFIX", "coredns")
LOG_LEVEL = os.getenv("LOG_LEVEL", "INFO").upper()