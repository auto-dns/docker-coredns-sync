from pydantic import BaseModel, Field, IPvAnyAddress, field_validator
from typing import Union
import re


def is_valid_hostname(hostname: str) -> bool:
    if len(hostname) > 255:
        return False
    pattern = r"^(?=.{1,255}$)[a-zA-Z0-9](?:[a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?(?:\.[a-zA-Z0-9\-]{1,63})*$"
    return re.match(pattern, hostname) is not None


class DnsRecord(BaseModel):
    name: str
    owner: str
    record_type: str
    value: str

    def render(self) -> str:
        return f"{self.record_type}: {self.name} -> {self.value} ({self.owner})"

    model_config = {
        "frozen": True,  # like dataclasses' frozen=True
        "extra": "forbid"  # prevent unknown fields
    }


class ARecord(DnsRecord):
    record_type: str = Field(default="A", frozen=True)
    value: IPvAnyAddress

    @field_validator("name")
    @classmethod
    def validate_hostname(cls, name: str) -> str:
        if not is_valid_hostname(name):
            raise ValueError(f"Invalid hostname for A record: {name}")
        return name


class CNAMERecord(DnsRecord):
    record_type: str = Field(default="CNAME", frozen=True)

    @field_validator("name", "value")
    @classmethod
    def validate_hostname(cls, v: str, field) -> str:
        if not is_valid_hostname(v):
            raise ValueError(f"Invalid {field.name} for CNAME record: {v}")
        return v


Record = Union[ARecord, CNAMERecord]
