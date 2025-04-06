# Labels
class RecordLabelError(ValueError):
    """Raised when a Docker label used for a DNS record is malformed or incomplete."""

class UnsupportedRecordTypeError(RecordLabelError):
    """Raised when a Docker label used for a DNS record contains an unsupported record type."""

# Etcd
class EtcdConnectionError(ConnectionError):
    """Raised when the Etcd client cannot connect to the Etcd server"""

# Registry
class RegistryUnsupportedRecordTypeError(ValueError):
    """Raised when the registry is given an unsupported record type"""

class RegistryParseError(ValueError):
    """Raised when a registry entry can't be parsed correctly."""

# Record Validator
class RecordValidationError(ValueError):
    """Raised when a new record creates a conflicting DNS state"""