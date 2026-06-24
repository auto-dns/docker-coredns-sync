package core

import (
	"testing"

	"github.com/auto-dns/docker-coredns-sync/internal/domain"
)

func TestParseLabels_EnabledTrue(t *testing.T) {
	labels := map[string]string{
		"coredns.enabled": "true",
	}

	result := ParseLabels("coredns", labels)

	if !result.Enabled {
		t.Error("expected Enabled to be true")
	}
}

func TestParseLabels_EnabledFalse(t *testing.T) {
	labels := map[string]string{
		"coredns.enabled": "false",
	}

	result := ParseLabels("coredns", labels)

	if result.Enabled {
		t.Error("expected Enabled to be false")
	}
}

func TestParseLabels_EnabledMissing(t *testing.T) {
	labels := map[string]string{}

	result := ParseLabels("coredns", labels)

	if result.Enabled {
		t.Error("expected Enabled to be false when missing")
	}
}

func TestParseLabels_EnabledCaseInsensitive(t *testing.T) {
	testCases := []string{"TRUE", "True", "TrUe", "true"}

	for _, val := range testCases {
		t.Run(val, func(t *testing.T) {
			labels := map[string]string{
				"coredns.enabled": val,
			}

			result := ParseLabels("coredns", labels)

			if !result.Enabled {
				t.Errorf("expected Enabled to be true for value %q", val)
			}
		})
	}
}

func TestParseLabels_ContainerForce(t *testing.T) {
	labels := map[string]string{
		"coredns.enabled": "true",
		"coredns.force":   "true",
	}

	result := ParseLabels("coredns", labels)

	if !result.ContainerForce {
		t.Error("expected ContainerForce to be true")
	}
}

func TestParseLabels_ContainerForceNotSet(t *testing.T) {
	labels := map[string]string{
		"coredns.enabled": "true",
	}

	result := ParseLabels("coredns", labels)

	if result.ContainerForce {
		t.Error("expected ContainerForce to be false when not set")
	}
}

func TestParseLabels_SimpleARecord(t *testing.T) {
	labels := map[string]string{
		"coredns.enabled": "true",
		"coredns.A.name":  "app.example.com",
		"coredns.A.value": "192.168.1.1",
	}

	result := ParseLabels("coredns", labels)

	if len(result.Records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(result.Records))
	}

	rec := result.Records[0]
	if rec.Kind != domain.RecordA {
		t.Errorf("expected Kind RecordA, got %v", rec.Kind)
	}
	if rec.Name != "app.example.com" {
		t.Errorf("expected Name 'app.example.com', got %q", rec.Name)
	}
	if rec.Value != "192.168.1.1" {
		t.Errorf("expected Value '192.168.1.1', got %q", rec.Value)
	}
	if rec.Alias != "" {
		t.Errorf("expected empty Alias, got %q", rec.Alias)
	}
}

func TestParseLabels_ARecordNameOnly(t *testing.T) {
	labels := map[string]string{
		"coredns.enabled": "true",
		"coredns.A.name":  "app.example.com",
	}

	result := ParseLabels("coredns", labels)

	if len(result.Records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(result.Records))
	}

	rec := result.Records[0]
	if rec.Value != "" {
		t.Errorf("expected empty Value, got %q", rec.Value)
	}
}

func TestParseLabels_ARecordWithAlias(t *testing.T) {
	labels := map[string]string{
		"coredns.enabled":     "true",
		"coredns.A.web.name":  "web.example.com",
		"coredns.A.web.value": "192.168.1.1",
	}

	result := ParseLabels("coredns", labels)

	if len(result.Records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(result.Records))
	}

	rec := result.Records[0]
	if rec.Alias != "web" {
		t.Errorf("expected Alias 'web', got %q", rec.Alias)
	}
	if rec.Name != "web.example.com" {
		t.Errorf("expected Name 'web.example.com', got %q", rec.Name)
	}
}

func TestParseLabels_AAAARecord(t *testing.T) {
	labels := map[string]string{
		"coredns.enabled":    "true",
		"coredns.AAAA.name":  "app.example.com",
		"coredns.AAAA.value": "::1",
	}

	result := ParseLabels("coredns", labels)

	if len(result.Records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(result.Records))
	}

	rec := result.Records[0]
	if rec.Kind != domain.RecordAAAA {
		t.Errorf("expected Kind RecordAAAA, got %v", rec.Kind)
	}
}

func TestParseLabels_CNAMERecord(t *testing.T) {
	labels := map[string]string{
		"coredns.enabled":     "true",
		"coredns.CNAME.name":  "alias.example.com",
		"coredns.CNAME.value": "target.example.com",
	}

	result := ParseLabels("coredns", labels)

	if len(result.Records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(result.Records))
	}

	rec := result.Records[0]
	if rec.Kind != domain.RecordCNAME {
		t.Errorf("expected Kind RecordCNAME, got %v", rec.Kind)
	}
}

func TestParseLabels_MultipleRecords(t *testing.T) {
	labels := map[string]string{
		"coredns.enabled":     "true",
		"coredns.A.name":      "app.example.com",
		"coredns.A.value":     "192.168.1.1",
		"coredns.AAAA.name":   "app.example.com",
		"coredns.AAAA.value":  "::1",
		"coredns.CNAME.name":  "alias.example.com",
		"coredns.CNAME.value": "app.example.com",
	}

	result := ParseLabels("coredns", labels)

	if len(result.Records) != 3 {
		t.Fatalf("expected 3 records, got %d", len(result.Records))
	}

	kinds := make(map[domain.RecordKind]bool)
	for _, rec := range result.Records {
		kinds[rec.Kind] = true
	}

	if !kinds[domain.RecordA] {
		t.Error("expected to find an A record")
	}
	if !kinds[domain.RecordAAAA] {
		t.Error("expected to find an AAAA record")
	}
	if !kinds[domain.RecordCNAME] {
		t.Error("expected to find a CNAME record")
	}
}

func TestParseLabels_MultipleAliases(t *testing.T) {
	labels := map[string]string{
		"coredns.enabled":     "true",
		"coredns.A.web.name":  "web.example.com",
		"coredns.A.web.value": "192.168.1.1",
		"coredns.A.api.name":  "api.example.com",
		"coredns.A.api.value": "192.168.1.2",
	}

	result := ParseLabels("coredns", labels)

	if len(result.Records) != 2 {
		t.Fatalf("expected 2 records, got %d", len(result.Records))
	}

	aliases := make(map[string]bool)
	for _, rec := range result.Records {
		aliases[rec.Alias] = true
	}

	if !aliases["web"] {
		t.Error("expected to find record with alias 'web'")
	}
	if !aliases["api"] {
		t.Error("expected to find record with alias 'api'")
	}
}

func TestParseLabels_RecordLevelTTL(t *testing.T) {
	labels := map[string]string{
		"coredns.enabled": "true",
		"coredns.A.name":  "app.example.com",
		"coredns.A.value": "192.168.1.1",
		"coredns.A.ttl":   "300",
	}

	result := ParseLabels("coredns", labels)

	if len(result.Records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(result.Records))
	}
	rec := result.Records[0]
	if rec.TTL == nil {
		t.Fatal("expected TTL to be set")
	}
	if *rec.TTL != 300 {
		t.Errorf("expected TTL 300, got %d", *rec.TTL)
	}
}

func TestParseLabels_RecordLevelTTLWithAlias(t *testing.T) {
	labels := map[string]string{
		"coredns.enabled":     "true",
		"coredns.A.web.name":  "web.example.com",
		"coredns.A.web.value": "192.168.1.1",
		"coredns.A.web.ttl":   "60",
	}

	result := ParseLabels("coredns", labels)

	if len(result.Records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(result.Records))
	}
	rec := result.Records[0]
	if rec.TTL == nil || *rec.TTL != 60 {
		t.Errorf("expected TTL 60, got %v", rec.TTL)
	}
}

func TestParseLabels_TTLNotSetIsNil(t *testing.T) {
	labels := map[string]string{
		"coredns.enabled": "true",
		"coredns.A.name":  "app.example.com",
		"coredns.A.value": "192.168.1.1",
	}

	result := ParseLabels("coredns", labels)

	if len(result.Records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(result.Records))
	}
	if result.Records[0].TTL != nil {
		t.Errorf("expected TTL to be nil when unspecified, got %v", *result.Records[0].TTL)
	}
}

func TestParseLabels_InvalidTTLIgnored(t *testing.T) {
	for _, val := range []string{"abc", "-5", "3.5", ""} {
		t.Run(val, func(t *testing.T) {
			labels := map[string]string{
				"coredns.enabled": "true",
				"coredns.A.name":  "app.example.com",
				"coredns.A.value": "192.168.1.1",
				"coredns.A.ttl":   val,
			}

			result := ParseLabels("coredns", labels)

			if len(result.Records) != 1 {
				t.Fatalf("expected 1 record, got %d", len(result.Records))
			}
			if result.Records[0].TTL != nil {
				t.Errorf("expected invalid TTL %q to be ignored (nil), got %v", val, *result.Records[0].TTL)
			}
		})
	}
}

func TestParseLabels_RecordLevelForce(t *testing.T) {
	labels := map[string]string{
		"coredns.enabled":       "true",
		"coredns.A.web.name":    "web.example.com",
		"coredns.A.web.value":   "192.168.1.1",
		"coredns.A.web.force":   "true",
		"coredns.A.api.name":    "api.example.com",
		"coredns.A.api.value":   "192.168.1.2",
		"coredns.A.other.name":  "other.example.com",
		"coredns.A.other.value": "192.168.1.3",
		"coredns.A.other.force": "false",
	}

	result := ParseLabels("coredns", labels)

	if len(result.Records) != 3 {
		t.Fatalf("expected 3 records, got %d", len(result.Records))
	}

	for _, rec := range result.Records {
		switch rec.Alias {
		case "web":
			if rec.Force == nil || !*rec.Force {
				t.Error("expected web record Force to be true")
			}
		case "api":
			if rec.Force != nil {
				t.Error("expected api record Force to be nil (not set)")
			}
		case "other":
			if rec.Force == nil || *rec.Force {
				t.Error("expected other record Force to be false")
			}
		}
	}
}

func TestParseLabels_IgnoresUnrelatedLabels(t *testing.T) {
	labels := map[string]string{
		"coredns.enabled":       "true",
		"coredns.A.name":        "app.example.com",
		"traefik.enable":        "true",
		"com.example.something": "value",
		"other.A.name":          "ignored.example.com",
	}

	result := ParseLabels("coredns", labels)

	if len(result.Records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(result.Records))
	}
}

func TestParseLabels_IgnoresUnknownKinds(t *testing.T) {
	labels := map[string]string{
		"coredns.enabled":  "true",
		"coredns.MX.name":  "mail.example.com",
		"coredns.MX.value": "mailserver.example.com",
		"coredns.TXT.name": "example.com",
		"coredns.TXT.text": "v=spf1 include:_spf.example.com ~all",
	}

	result := ParseLabels("coredns", labels)

	if len(result.Records) != 0 {
		t.Fatalf("expected 0 records (unknown kinds), got %d", len(result.Records))
	}
}

func TestParseLabels_MalformedLabels(t *testing.T) {
	labels := map[string]string{
		"coredns.enabled": "true",
		"coredns.A":       "incomplete",        // Missing .name or .value
		"coredns":         "root_only",         // Just prefix
		"coredns.":        "trailing_dot",      // Trailing dot
		"coredns.A.name":  "valid.example.com", // This one is valid
	}

	result := ParseLabels("coredns", labels)

	// Should have 1 valid record
	if len(result.Records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(result.Records))
	}
}

func TestParseLabels_CustomPrefix(t *testing.T) {
	labels := map[string]string{
		"mydns.enabled": "true",
		"mydns.A.name":  "app.example.com",
		"mydns.A.value": "192.168.1.1",
	}

	result := ParseLabels("mydns", labels)

	if !result.Enabled {
		t.Error("expected Enabled to be true with custom prefix")
	}
	if len(result.Records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(result.Records))
	}
}

func TestParseLabels_WhitespaceHandling(t *testing.T) {
	labels := map[string]string{
		"coredns.enabled": "true",
		"coredns.A.name":  "  app.example.com  ",
		"coredns.A.value": "  192.168.1.1  ",
	}

	result := ParseLabels("coredns", labels)

	if len(result.Records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(result.Records))
	}

	rec := result.Records[0]
	if rec.Name != "app.example.com" {
		t.Errorf("expected trimmed Name 'app.example.com', got %q", rec.Name)
	}
	if rec.Value != "192.168.1.1" {
		t.Errorf("expected trimmed Value '192.168.1.1', got %q", rec.Value)
	}
}

func TestParseLabels_CaseInsensitiveKind(t *testing.T) {
	testCases := []struct {
		kindLabel    string
		expectedKind domain.RecordKind
	}{
		{"a", domain.RecordA},
		{"A", domain.RecordA},
		{"aaaa", domain.RecordAAAA},
		{"AAAA", domain.RecordAAAA},
		{"Aaaa", domain.RecordAAAA},
		{"cname", domain.RecordCNAME},
		{"CNAME", domain.RecordCNAME},
		{"Cname", domain.RecordCNAME},
	}

	for _, tc := range testCases {
		t.Run(tc.kindLabel, func(t *testing.T) {
			labels := map[string]string{
				"coredns.enabled":                    "true",
				"coredns." + tc.kindLabel + ".name":  "app.example.com",
				"coredns." + tc.kindLabel + ".value": "target.example.com",
			}

			result := ParseLabels("coredns", labels)

			if len(result.Records) != 1 {
				t.Fatalf("expected 1 record, got %d", len(result.Records))
			}

			if result.Records[0].Kind != tc.expectedKind {
				t.Errorf("expected kind %v, got %v", tc.expectedKind, result.Records[0].Kind)
			}
		})
	}
}

func TestLabeledRecord_GetNameLabel(t *testing.T) {
	tests := []struct {
		name     string
		prefix   string
		kind     domain.RecordKind
		alias    string
		expected string
	}{
		{"A without alias", "coredns", domain.RecordA, "", "coredns.A.name"},
		{"A with alias", "coredns", domain.RecordA, "web", "coredns.A.web.name"},
		{"AAAA without alias", "coredns", domain.RecordAAAA, "", "coredns.AAAA.name"},
		{"CNAME with alias", "coredns", domain.RecordCNAME, "alias1", "coredns.CNAME.alias1.name"},
		{"custom prefix", "mydns", domain.RecordA, "app", "mydns.A.app.name"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lr := LabeledRecord{
				prefix: tt.prefix,
				Kind:   tt.kind,
				Alias:  tt.alias,
			}

			result := lr.GetNameLabel()

			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestLabeledRecord_GetValueLabel(t *testing.T) {
	tests := []struct {
		name     string
		prefix   string
		kind     domain.RecordKind
		alias    string
		expected string
	}{
		{"A without alias", "coredns", domain.RecordA, "", "coredns.A.value"},
		{"A with alias", "coredns", domain.RecordA, "web", "coredns.A.web.value"},
		{"AAAA without alias", "coredns", domain.RecordAAAA, "", "coredns.AAAA.value"},
		{"CNAME with alias", "coredns", domain.RecordCNAME, "alias1", "coredns.CNAME.alias1.value"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lr := LabeledRecord{
				prefix: tt.prefix,
				Kind:   tt.kind,
				Alias:  tt.alias,
			}

			result := lr.GetValueLabel()

			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestBoolFromLabel(t *testing.T) {
	trueValues := []string{"true", "TRUE", "True", "TrUe", " true ", "  TRUE  "}
	falseValues := []string{"false", "FALSE", "False", "", "0", "1", "yes", "no"}

	for _, v := range trueValues {
		t.Run("true:"+v, func(t *testing.T) {
			if !boolFromLabel(v) {
				t.Errorf("expected %q to be true", v)
			}
		})
	}

	for _, v := range falseValues {
		t.Run("false:"+v, func(t *testing.T) {
			if boolFromLabel(v) {
				t.Errorf("expected %q to be false", v)
			}
		})
	}
}

func TestParseLabels_InvalidFieldSkipped(t *testing.T) {
	labels := map[string]string{
		"coredns.enabled":       "true",
		"coredns.A.web.name":    "app.example.com",
		"coredns.A.web.value":   "192.168.1.1",
		"coredns.A.web.invalid": "should-be-ignored",
		"coredns.A.web.other":   "also-ignored",
	}

	result := ParseLabels("coredns", labels)

	if len(result.Records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(result.Records))
	}

	rec := result.Records[0]
	if rec.Name != "app.example.com" {
		t.Errorf("expected name 'app.example.com', got %q", rec.Name)
	}
	if rec.Value != "192.168.1.1" {
		t.Errorf("expected value '192.168.1.1', got %q", rec.Value)
	}
}

func TestParseLabels_OnlyInvalidFieldsNoRecord(t *testing.T) {
	labels := map[string]string{
		"coredns.enabled":       "true",
		"coredns.A.web.invalid": "ignored",
		"coredns.A.web.other":   "also-ignored",
	}

	result := ParseLabels("coredns", labels)

	// Invalid fields should not create records
	if len(result.Records) != 0 {
		t.Errorf("expected 0 records for invalid fields only, got %d", len(result.Records))
	}
}
