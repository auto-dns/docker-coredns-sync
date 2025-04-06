from datetime import datetime, timezone
from pydantic import BaseModel, Field
from core.dns_record import Record

class RecordIntent(BaseModel):
	container_id: str
	container_name: str
	created: datetime = Field(default_factory=lambda: datetime.now(timezone.utc))
	force: bool = Field(default=False)
	hostname: str
	record: Record
