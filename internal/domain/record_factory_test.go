package domain

import (
	"testing"
)

func TestParseKind_A(t *testing.T) {
	tests := []string{"A", "a"}

	for _, input := range tests {
		t.Run(input, func(t *testing.T) {
			kind, err := ParseKind(input)

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if kind != RecordA {
				t.Errorf("expected RecordA, got %v", kind)
			}
		})
	}
}

func TestParseKind_AAAA(t *testing.T) {
	tests := []string{"AAAA", "aaaa", "Aaaa"}

	for _, input := range tests {
		t.Run(input, func(t *testing.T) {
			kind, err := ParseKind(input)

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if kind != RecordAAAA {
				t.Errorf("expected RecordAAAA, got %v", kind)
			}
		})
	}
}

func TestParseKind_CNAME(t *testing.T) {
	tests := []string{"CNAME", "cname", "Cname", "CName"}

	for _, input := range tests {
		t.Run(input, func(t *testing.T) {
			kind, err := ParseKind(input)

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if kind != RecordCNAME {
				t.Errorf("expected RecordCNAME, got %v", kind)
			}
		})
	}
}

func TestParseKind_Unknown(t *testing.T) {
	unknownKinds := []string{"MX", "TXT", "NS", "SOA", "PTR", "SRV", "", "unknown", "B"}

	for _, input := range unknownKinds {
		t.Run(input, func(t *testing.T) {
			_, err := ParseKind(input)

			if err == nil {
				t.Errorf("expected error for unknown kind %q", input)
			}
		})
	}
}

func TestNewFromKind_A(t *testing.T) {
	rec, err := NewFromKind(RecordA, "app.example.com", "192.168.1.1")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Kind != RecordA {
		t.Errorf("expected RecordA, got %v", rec.Kind)
	}
	if rec.Name != "app.example.com" {
		t.Errorf("expected name 'app.example.com', got %q", rec.Name)
	}
	if rec.Value != "192.168.1.1" {
		t.Errorf("expected value '192.168.1.1', got %q", rec.Value)
	}
}

func TestNewFromKind_AAAA(t *testing.T) {
	rec, err := NewFromKind(RecordAAAA, "app.example.com", "::1")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Kind != RecordAAAA {
		t.Errorf("expected RecordAAAA, got %v", rec.Kind)
	}
}

func TestNewFromKind_CNAME(t *testing.T) {
	rec, err := NewFromKind(RecordCNAME, "alias.example.com", "target.example.com")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Kind != RecordCNAME {
		t.Errorf("expected RecordCNAME, got %v", rec.Kind)
	}
}

func TestNewFromKind_InvalidKind(t *testing.T) {
	_, err := NewFromKind(RecordKind("INVALID"), "app.example.com", "192.168.1.1")

	if err == nil {
		t.Error("expected error for invalid kind")
	}
}

func TestNewFromKind_InvalidValue(t *testing.T) {
	// A record with invalid IP
	_, err := NewFromKind(RecordA, "app.example.com", "not-an-ip")

	if err == nil {
		t.Error("expected error for invalid A record value")
	}
}

func TestNewFromString_A(t *testing.T) {
	rec, err := NewFromString("A", "app.example.com", "192.168.1.1")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Kind != RecordA {
		t.Errorf("expected RecordA, got %v", rec.Kind)
	}
	if rec.Name != "app.example.com" {
		t.Errorf("expected name 'app.example.com', got %q", rec.Name)
	}
	if rec.Value != "192.168.1.1" {
		t.Errorf("expected value '192.168.1.1', got %q", rec.Value)
	}
}

func TestNewFromString_CaseInsensitive(t *testing.T) {
	tests := []struct {
		kind     string
		expected RecordKind
	}{
		{"a", RecordA},
		{"A", RecordA},
		{"aaaa", RecordAAAA},
		{"AAAA", RecordAAAA},
		{"cname", RecordCNAME},
		{"CNAME", RecordCNAME},
	}

	for _, tt := range tests {
		t.Run(tt.kind, func(t *testing.T) {
			var value string
			var name string = "app.example.com"

			switch tt.expected {
			case RecordA:
				value = "192.168.1.1"
			case RecordAAAA:
				value = "::1"
			case RecordCNAME:
				value = "target.example.com"
			}

			rec, err := NewFromString(tt.kind, name, value)

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if rec.Kind != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, rec.Kind)
			}
		})
	}
}

func TestNewFromString_UnknownKind(t *testing.T) {
	_, err := NewFromString("MX", "app.example.com", "mail.example.com")

	if err == nil {
		t.Error("expected error for unknown kind")
	}
}

func TestNewFromString_InvalidValue(t *testing.T) {
	_, err := NewFromString("A", "app.example.com", "not-an-ip")

	if err == nil {
		t.Error("expected error for invalid value")
	}
}

func TestNewFromString_InvalidHostname(t *testing.T) {
	_, err := NewFromString("A", "", "192.168.1.1")

	if err == nil {
		t.Error("expected error for invalid hostname")
	}
}
