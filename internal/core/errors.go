package core

// RecordValidationError represents an error that occurs during DNS record validation
type RecordValidationError struct {
	Message string
}

// Error implements the error interface
func (e *RecordValidationError) Error() string {
	return e.Message
}

// NewRecordValidationError creates a new RecordValidationError
func NewRecordValidationError(message string) *RecordValidationError {
	return &RecordValidationError{Message: message}
}
