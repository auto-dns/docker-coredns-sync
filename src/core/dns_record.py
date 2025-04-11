import re
from ipaddress import ip_address
from typing import Union

from pydantic import BaseModel, Field, ValidationInfo, field_validator


def is_valid_hostname(hostname: str) -> bool:
    if len(hostname) > 255:
        return False
    pattern = (
        r"^(?=.{1,255}$)[a-zA-Z0-9](?:[a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?"
        r"(?:\.[a-zA-Z0-9\-]{1,63})*$"
    )
    return re.match(pattern, hostname) is not None


class DnsRecord(BaseModel):
    name: str
    record_type: str
    value: str

    def render(self) -> str:
        return f"{self.name} -> {getattr(self, 'value', '<no value>')}"

    def __eq__(self, other: object) -> bool:
        if not isinstance(other, DnsRecord):
            return NotImplemented
        return (self.name == other.name and 
                self.record_type == other.record_type and 
                self.value == other.value)

    def __hash__(self) -> int:
        return hash((self.name, self.record_type, self.value))

    model_config = {
        "frozen": True,
        "extra": "forbid",
    }


class ARecord(DnsRecord):
    record_type: str = Field(default="A", frozen=True)

    @field_validator("name")
    @classmethod
    def validate_hostname(cls, name: str) -> str:
        if not is_valid_hostname(name):
            raise ValueError(f"Invalid hostname for A record: {name}") from None
        return name

    @field_validator("value")
    @classmethod
    def validate_ip(cls, value: str) -> str:
        try:
            ip_address(value)
            return value
        except ValueError:
            raise ValueError(f"Invalid IP address: {value}") from None


class CNAMERecord(DnsRecord):
    record_type: str = Field(default="CNAME", frozen=True)

    @field_validator("name", "value")
    @classmethod
    def validate_hostname(cls, v: str, info: ValidationInfo) -> str:
        if not is_valid_hostname(v):
            raise ValueError(f"Invalid {info.field_name} for CNAME record: {v}") from None
        return v


Record = Union[ARecord, CNAMERecord]