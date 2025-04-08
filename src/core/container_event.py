from datetime import datetime, timezone
from typing import Any, Dict, Optional


class ContainerEvent:
    def __init__(
        self,
        id: Optional[str],
        name: Optional[str] = None,
        created: Optional[datetime] = None,
        status: str = "",
        labels: Optional[Dict[str, str]] = None,
        attrs: Optional[Dict[str, Any]] = None,
    ) -> None:
        self.id: str = id or "<unknown>"
        self.name: str = name or "<unknown>"
        self.created: datetime = created or datetime.now(timezone.utc)
        self.status: str = status
        self.labels: Dict[str, str] = labels or {}
        self.attrs: Dict[str, Any] = attrs or {}