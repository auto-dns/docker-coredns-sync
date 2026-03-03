package registry

import (
	"testing"
)

func TestKeyBaseForFQDN_Simple(t *testing.T) {
	result := keyBaseForFQDN("/skydns", "example.com")

	expected := "/skydns/com/example"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestKeyBaseForFQDN_Subdomain(t *testing.T) {
	result := keyBaseForFQDN("/skydns", "app.example.com")

	expected := "/skydns/com/example/app"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestKeyBaseForFQDN_DeepSubdomain(t *testing.T) {
	result := keyBaseForFQDN("/skydns", "a.b.c.example.com")

	expected := "/skydns/com/example/c/b/a"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestKeyBaseForFQDN_TrailingDot(t *testing.T) {
	result := keyBaseForFQDN("/skydns", "example.com.")

	expected := "/skydns/com/example"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestKeyBaseForFQDN_PrefixWithTrailingSlash(t *testing.T) {
	result := keyBaseForFQDN("/skydns/", "example.com")

	expected := "/skydns/com/example"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestKeyBaseForFQDN_PrefixWithMultipleTrailingSlashes(t *testing.T) {
	result := keyBaseForFQDN("/skydns///", "example.com")

	// Trims trailing slashes
	expected := "/skydns/com/example"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestKeyBaseForFQDN_SingleLabel(t *testing.T) {
	result := keyBaseForFQDN("/skydns", "localhost")

	expected := "/skydns/localhost"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestKeyBaseForFQDN_CustomPrefix(t *testing.T) {
	result := keyBaseForFQDN("/dns/local", "app.example.com")

	expected := "/dns/local/com/example/app"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestKeyBaseForFQDN_WhitespaceInFQDN(t *testing.T) {
	result := keyBaseForFQDN("/skydns", "  example.com  ")

	expected := "/skydns/com/example"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestFqdnFromKey_Simple(t *testing.T) {
	result := fqdnFromKey("/skydns", "/skydns/com/example/x1")

	expected := "example.com"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestFqdnFromKey_Subdomain(t *testing.T) {
	result := fqdnFromKey("/skydns", "/skydns/com/example/app/x1")

	expected := "app.example.com"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestFqdnFromKey_DeepSubdomain(t *testing.T) {
	result := fqdnFromKey("/skydns", "/skydns/com/example/c/b/a/x1")

	expected := "a.b.c.example.com"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestFqdnFromKey_NoIndex(t *testing.T) {
	// Key without xN suffix - should still work
	result := fqdnFromKey("/skydns", "/skydns/com/example")

	expected := "example.com"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestFqdnFromKey_DifferentIndex(t *testing.T) {
	tests := []struct {
		key      string
		expected string
	}{
		{"/skydns/com/example/x1", "example.com"},
		{"/skydns/com/example/x2", "example.com"},
		{"/skydns/com/example/x99", "example.com"},
		{"/skydns/com/example/x123", "example.com"},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			result := fqdnFromKey("/skydns", tt.key)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestFqdnFromKey_SingleLabel(t *testing.T) {
	result := fqdnFromKey("/skydns", "/skydns/localhost/x1")

	expected := "localhost"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestFqdnFromKey_CustomPrefix(t *testing.T) {
	result := fqdnFromKey("/dns/local", "/dns/local/com/example/app/x1")

	expected := "app.example.com"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestFqdnFromKey_PrefixWithTrailingSlash(t *testing.T) {
	result := fqdnFromKey("/skydns/", "/skydns/com/example/x1")

	expected := "example.com"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestKeyBaseForFQDN_AndFqdnFromKey_Roundtrip(t *testing.T) {
	fqdns := []string{
		"example.com",
		"app.example.com",
		"a.b.c.example.com",
		"localhost",
		"deep.nested.subdomain.example.org",
	}

	for _, fqdn := range fqdns {
		t.Run(fqdn, func(t *testing.T) {
			base := keyBaseForFQDN("/skydns", fqdn)
			key := base + "/x1"
			result := fqdnFromKey("/skydns", key)

			if result != fqdn {
				t.Errorf("roundtrip failed: started with %q, got %q", fqdn, result)
			}
		})
	}
}
