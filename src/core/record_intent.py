from datetime import datetime, timezone

from pydantic import BaseModel, Field

from src.core.dns_record import Record


class RecordIntent(BaseModel):
    container_id: str = "<unknown>"
    container_name: str = "<unknown>"
    created: datetime = Field(default_factory=lambda: datetime.now(timezone.utc))
    force: bool = False
    hostname: str
    record: Record
