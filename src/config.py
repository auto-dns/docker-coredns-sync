import os

# General
HOST_IP = os.getenv("HOST_IP", "127.0.0.1")
HOSTNAME = os.getenv("HOSTNAME", "your-hostname")
DOCKER_LABEL_PREFIX = os.getenv("DOCKER_LABEL_PREFIX", "coredns")
LOG_LEVEL = os.getenv("LOG_LEVEL", "INFO").upper()

# etcd
ETCD_HOST = os.getenv("ETCD_HOST", "localhost")
ETCD_PORT = int(os.getenv("ETCD_PORT", 2379))
ETCD_PATH_PREFIX = os.getenv("ETCD_PATH_PREFIX", "/skydns")
