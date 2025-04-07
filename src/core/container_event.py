from datetime import datetime
from typing import Optional, Dict


class ContainerEvent:
    def __init__(
        self,
        id: str,
        name: Optional[str] = None,
        created: Optional[datetime] = None,
        status: str = "",
        labels: Optional[Dict[str, str]] = None,
        attrs: Optional[Dict] = None,
    ):
        self.id = id
        self.name = name or "<unknown>"
        self.created = created
        self.status = status
        self.labels = labels or {}
        self.attrs = attrs or {}
