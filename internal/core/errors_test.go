package core

import (
	"strings"
	"testing"
)

func TestRecordValidationError_Error(t *testing.T) {
	err := NewRecordValidationError("test error message")

	if err.Error() != "test error message" {
		t.Errorf("expected 'test error message', got %q", err.Error())
	}
}

func TestRecordValidationError_Message(t *testing.T) {
	err := NewRecordValidationError("custom message")

	if err.Message != "custom message" {
		t.Errorf("expected Message 'custom message', got %q", err.Message)
	}
}

func TestRecordValidationError_ImplementsError(t *testing.T) {
	var _ error = &RecordValidationError{}
}

func TestRecordValidationError_EmptyMessage(t *testing.T) {
	err := NewRecordValidationError("")

	if err.Error() != "" {
		t.Errorf("expected empty error message, got %q", err.Error())
	}
}

func TestRecordValidationError_WithDetails(t *testing.T) {
	msg := "cannot add A record: CNAME exists for app.example.com"
	err := NewRecordValidationError(msg)

	if !strings.Contains(err.Error(), "CNAME") {
		t.Error("expected error message to contain 'CNAME'")
	}
	if !strings.Contains(err.Error(), "app.example.com") {
		t.Error("expected error message to contain 'app.example.com'")
	}
}
