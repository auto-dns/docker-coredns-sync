from pydantic import BaseSettings, Field


class Settings(BaseSettings):
    # General
    allowed_record_types: list = ['A', 'CNAME']
    docker_label_prefix: str = Field(default="coredns")
    host_ip: str = Field(default="127.0.0.1")
    hostname: str = Field(default="your-hostname")
    log_level: str = Field(default="INFO")

    # etcd
    etcd_host: str = Field(default="localhost")
    etcd_port: int = Field(default=2379)
    etcd_path_prefix: str = Field(default="/skydns")
    etcd_lock_ttl: float = Field(default=5)
    etcd_lock_timeout: float = Field(default=2.0)
    etcd_lock_retry_interval: float = Field(default=0.1)

    model_config = {
        "env_file": ".env",  # or .devcontainer/devcontainer.env
        "extra": "ignore"
    }


def load_settings() -> Settings:
    return Settings()
