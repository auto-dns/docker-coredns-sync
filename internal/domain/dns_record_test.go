package domain

import (
	"testing"
)

func TestNewA_ValidInput(t *testing.T) {
	tests := []struct {
		name     string
		hostname string
		ipv4     string
	}{
		{"simple hostname", "example.com", "192.168.1.1"},
		{"subdomain", "app.example.com", "10.0.0.1"},
		{"deep subdomain", "a.b.c.example.com", "172.16.0.1"},
		{"single label", "localhost", "127.0.0.1"},
		{"with hyphens", "my-app.example.com", "8.8.8.8"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec, err := NewA(tt.hostname, tt.ipv4)

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if rec.Name != tt.hostname {
				t.Errorf("expected Name %q, got %q", tt.hostname, rec.Name)
			}
			if rec.Kind != RecordA {
				t.Errorf("expected Kind RecordA, got %v", rec.Kind)
			}
			if rec.Value != tt.ipv4 {
				t.Errorf("expected Value %q, got %q", tt.ipv4, rec.Value)
			}
		})
	}
}

func TestNewA_InvalidHostname(t *testing.T) {
	invalidHostnames := []string{
		"",
		"-startswithhyphen.com",
		"endswithhyphen-.com",
		"has spaces.com",
		"has_underscore.com",
		"too" + string(make([]byte, 256)), // too long
	}

	for _, hostname := range invalidHostnames {
		t.Run(hostname, func(t *testing.T) {
			_, err := NewA(hostname, "192.168.1.1")

			if err == nil {
				t.Errorf("expected error for invalid hostname %q", hostname)
			}
		})
	}
}

func TestNewA_InvalidIPv4(t *testing.T) {
	tests := []struct {
		name string
		ip   string
	}{
		{"empty", ""},
		{"not an IP", "not-an-ip"},
		{"ipv6 address", "::1"},
		{"ipv6 full", "2001:db8::1"},
		{"partial IP", "192.168.1"},
		{"too many octets", "192.168.1.1.1"},
		{"out of range", "256.256.256.256"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewA("example.com", tt.ip)

			if err == nil {
				t.Errorf("expected error for invalid IPv4 %q", tt.ip)
			}
		})
	}
}

func TestNewAAAA_ValidInput(t *testing.T) {
	tests := []struct {
		name     string
		hostname string
		ipv6     string
	}{
		{"full ipv6", "example.com", "2001:0db8:85a3:0000:0000:8a2e:0370:7334"},
		{"compressed ipv6", "example.com", "2001:db8:85a3::8a2e:370:7334"},
		{"loopback", "localhost", "::1"},
		{"link local", "example.com", "fe80::1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec, err := NewAAAA(tt.hostname, tt.ipv6)

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if rec.Name != tt.hostname {
				t.Errorf("expected Name %q, got %q", tt.hostname, rec.Name)
			}
			if rec.Kind != RecordAAAA {
				t.Errorf("expected Kind RecordAAAA, got %v", rec.Kind)
			}
			if rec.Value != tt.ipv6 {
				t.Errorf("expected Value %q, got %q", tt.ipv6, rec.Value)
			}
		})
	}
}

func TestNewAAAA_InvalidHostname(t *testing.T) {
	_, err := NewAAAA("", "::1")

	if err == nil {
		t.Error("expected error for empty hostname")
	}
}

func TestNewAAAA_InvalidIPv6(t *testing.T) {
	tests := []struct {
		name string
		ip   string
	}{
		{"empty", ""},
		{"not an IP", "not-an-ip"},
		{"ipv4 address", "192.168.1.1"},
		{"partial ipv6", "2001:db8"},
		{"invalid chars", "2001:db8::gggg"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewAAAA("example.com", tt.ip)

			if err == nil {
				t.Errorf("expected error for invalid IPv6 %q", tt.ip)
			}
		})
	}
}

func TestNewCNAME_ValidInput(t *testing.T) {
	tests := []struct {
		name   string
		cname  string
		target string
	}{
		{"simple", "alias.example.com", "target.example.com"},
		{"subdomain to root", "www.example.com", "example.com"},
		{"cross domain", "app.mysite.com", "app.cloudprovider.net"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec, err := NewCNAME(tt.cname, tt.target)

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if rec.Name != tt.cname {
				t.Errorf("expected Name %q, got %q", tt.cname, rec.Name)
			}
			if rec.Kind != RecordCNAME {
				t.Errorf("expected Kind RecordCNAME, got %v", rec.Kind)
			}
			if rec.Value != tt.target {
				t.Errorf("expected Value %q, got %q", tt.target, rec.Value)
			}
		})
	}
}

func TestNewCNAME_InvalidName(t *testing.T) {
	_, err := NewCNAME("", "target.example.com")

	if err == nil {
		t.Error("expected error for empty CNAME name")
	}
}

func TestNewCNAME_InvalidTarget(t *testing.T) {
	_, err := NewCNAME("alias.example.com", "")

	if err == nil {
		t.Error("expected error for empty CNAME target")
	}
}

func TestRecord_Key_UniquePerRecord(t *testing.T) {
	r1, _ := NewA("app1.example.com", "192.168.1.1")
	r2, _ := NewA("app2.example.com", "192.168.1.1")
	r3, _ := NewA("app1.example.com", "192.168.1.2")
	r4, _ := NewAAAA("app1.example.com", "::1")
	r5, _ := NewCNAME("app1.example.com", "target.example.com")

	keys := map[string]bool{
		r1.Key(): true,
		r2.Key(): true,
		r3.Key(): true,
		r4.Key(): true,
		r5.Key(): true,
	}

	if len(keys) != 5 {
		t.Errorf("expected 5 unique keys, got %d", len(keys))
	}
}

func TestRecord_Key_SameForIdentical(t *testing.T) {
	r1, _ := NewA("app.example.com", "192.168.1.1")
	r2, _ := NewA("app.example.com", "192.168.1.1")

	if r1.Key() != r2.Key() {
		t.Errorf("expected identical records to have same key: %q vs %q", r1.Key(), r2.Key())
	}
}

func TestRecord_Equal_TrueForIdentical(t *testing.T) {
	r1, _ := NewA("app.example.com", "192.168.1.1")
	r2, _ := NewA("app.example.com", "192.168.1.1")

	if !r1.Equal(r2) {
		t.Error("expected identical records to be equal")
	}
}

func TestRecord_Equal_FalseForDifferentName(t *testing.T) {
	r1, _ := NewA("app1.example.com", "192.168.1.1")
	r2, _ := NewA("app2.example.com", "192.168.1.1")

	if r1.Equal(r2) {
		t.Error("expected records with different names to not be equal")
	}
}

func TestRecord_Equal_FalseForDifferentKind(t *testing.T) {
	r1 := Record{Name: "app.example.com", Kind: RecordA, Value: "192.168.1.1"}
	r2 := Record{Name: "app.example.com", Kind: RecordAAAA, Value: "192.168.1.1"}

	if r1.Equal(r2) {
		t.Error("expected records with different kinds to not be equal")
	}
}

func TestRecord_Equal_FalseForDifferentValue(t *testing.T) {
	r1, _ := NewA("app.example.com", "192.168.1.1")
	r2, _ := NewA("app.example.com", "192.168.1.2")

	if r1.Equal(r2) {
		t.Error("expected records with different values to not be equal")
	}
}

func TestRecord_Render_ARecord(t *testing.T) {
	r, _ := NewA("app.example.com", "192.168.1.1")

	rendered := r.Render()

	expected := "[A] app.example.com -> 192.168.1.1"
	if rendered != expected {
		t.Errorf("expected %q, got %q", expected, rendered)
	}
}

func TestRecord_Render_AAAARecord(t *testing.T) {
	r, _ := NewAAAA("app.example.com", "::1")

	rendered := r.Render()

	expected := "[AAAA] app.example.com -> ::1"
	if rendered != expected {
		t.Errorf("expected %q, got %q", expected, rendered)
	}
}

func TestRecord_Render_CNAMERecord(t *testing.T) {
	r, _ := NewCNAME("alias.example.com", "target.example.com")

	rendered := r.Render()

	expected := "[CNAME] alias.example.com -> target.example.com"
	if rendered != expected {
		t.Errorf("expected %q, got %q", expected, rendered)
	}
}

func TestRecord_Render_EmptyValue(t *testing.T) {
	r := Record{Name: "app.example.com", Kind: RecordA, Value: ""}

	rendered := r.Render()

	expected := "[A] app.example.com -> <no value>"
	if rendered != expected {
		t.Errorf("expected %q, got %q", expected, rendered)
	}
}

func TestRecord_IsA(t *testing.T) {
	a, _ := NewA("app.example.com", "192.168.1.1")
	aaaa, _ := NewAAAA("app.example.com", "::1")
	cname, _ := NewCNAME("app.example.com", "target.example.com")

	if !a.IsA() {
		t.Error("expected A record IsA() to be true")
	}
	if aaaa.IsA() {
		t.Error("expected AAAA record IsA() to be false")
	}
	if cname.IsA() {
		t.Error("expected CNAME record IsA() to be false")
	}
}

func TestRecord_IsAAAA(t *testing.T) {
	a, _ := NewA("app.example.com", "192.168.1.1")
	aaaa, _ := NewAAAA("app.example.com", "::1")
	cname, _ := NewCNAME("app.example.com", "target.example.com")

	if a.IsAAAA() {
		t.Error("expected A record IsAAAA() to be false")
	}
	if !aaaa.IsAAAA() {
		t.Error("expected AAAA record IsAAAA() to be true")
	}
	if cname.IsAAAA() {
		t.Error("expected CNAME record IsAAAA() to be false")
	}
}

func TestRecord_IsCNAME(t *testing.T) {
	a, _ := NewA("app.example.com", "192.168.1.1")
	aaaa, _ := NewAAAA("app.example.com", "::1")
	cname, _ := NewCNAME("app.example.com", "target.example.com")

	if a.IsCNAME() {
		t.Error("expected A record IsCNAME() to be false")
	}
	if aaaa.IsCNAME() {
		t.Error("expected AAAA record IsCNAME() to be false")
	}
	if !cname.IsCNAME() {
		t.Error("expected CNAME record IsCNAME() to be true")
	}
}

func TestRecord_IsAddress(t *testing.T) {
	a, _ := NewA("app.example.com", "192.168.1.1")
	aaaa, _ := NewAAAA("app.example.com", "::1")
	cname, _ := NewCNAME("app.example.com", "target.example.com")

	if !a.IsAddress() {
		t.Error("expected A record IsAddress() to be true")
	}
	if !aaaa.IsAddress() {
		t.Error("expected AAAA record IsAddress() to be true")
	}
	if cname.IsAddress() {
		t.Error("expected CNAME record IsAddress() to be false")
	}
}

func TestIsValidHostname_ValidCases(t *testing.T) {
	validHostnames := []string{
		"example.com",
		"sub.example.com",
		"a.b.c.d.example.com",
		"localhost",
		"my-app",
		"my-app.example.com",
		"app1",
		"1app",
		"123",
		"a",
		"a1b2c3",
	}

	for _, h := range validHostnames {
		t.Run(h, func(t *testing.T) {
			if !isValidHostname(h) {
				t.Errorf("expected %q to be valid", h)
			}
		})
	}
}

func TestIsValidHostname_InvalidCases(t *testing.T) {
	invalidHostnames := []string{
		"",
		"-starts-with-hyphen",
		"ends-with-hyphen-",
		"has..double.dots",
		".starts.with.dot",
		"ends.with.dot.",
		"has space",
		"has_underscore",
		"has@symbol",
		"has/slash",
	}

	for _, h := range invalidHostnames {
		t.Run(h, func(t *testing.T) {
			if isValidHostname(h) {
				t.Errorf("expected %q to be invalid", h)
			}
		})
	}
}

func TestIsValidHostname_MaxLength(t *testing.T) {
	// Create a 255-character hostname (max allowed)
	// Each label can be up to 63 chars, total max 255
	longLabel := "a"
	for i := 0; i < 62; i++ {
		longLabel += "a"
	}
	// 63 chars label + "." + 63 chars + "." + 63 chars + "." + 63 chars = 255
	maxHostname := longLabel + "." + longLabel + "." + longLabel + "." + "a" + longLabel[:62]

	if len(maxHostname) > 255 {
		t.Fatalf("test setup error: hostname is %d chars, expected <= 255", len(maxHostname))
	}

	// Just under the limit should work (if valid format)
	validLong := "a.example.com"
	if !isValidHostname(validLong) {
		t.Errorf("expected valid hostname %q to pass", validLong)
	}

	// Over the limit should fail
	tooLong := ""
	for len(tooLong) <= 256 {
		tooLong = tooLong + "a"
	}
	if isValidHostname(tooLong) {
		t.Error("expected hostname over 255 chars to be invalid")
	}
}
