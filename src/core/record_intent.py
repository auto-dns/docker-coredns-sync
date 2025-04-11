from datetime import datetime, timezone

from pydantic import BaseModel, Field

from src.core.dns_record import Record


class RecordIntent(BaseModel):
    container_id: str = "<unknown>"
    container_name: str = "<unknown>"
    created: datetime = Field(default_factory=lambda: datetime.now(timezone.utc))
    hostname: str
    force: bool = False
    record: Record

    def __eq__(self, other):
        if not isinstance(other, RecordIntent):
            return False
        
        return (
            self.container_id == other.container_id and
            self.container_name == other.container_name and
            self.hostname == other.hostname and
            self.force == other.force and
            self.record == other.record
        )
    
    def __hash__(self) -> int:
        return hash((
            self.container_id,
            self.container_name,
            self.hostname,
            self.force,
            self.record
        ))
