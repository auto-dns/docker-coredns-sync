package config

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// writeSelfSignedPEMs generates a self-signed certificate and writes the cert,
// key, and a CA file (the same self-signed cert) to a temp dir.
func writeSelfSignedPEMs(t *testing.T) (certFile, keyFile, caFile string) {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	tmpl := x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "test"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &priv.PublicKey, priv)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)})

	dir := t.TempDir()
	certFile = filepath.Join(dir, "cert.pem")
	keyFile = filepath.Join(dir, "key.pem")
	caFile = filepath.Join(dir, "ca.pem")
	for path, content := range map[string][]byte{certFile: certPEM, keyFile: keyPEM, caFile: certPEM} {
		if err := os.WriteFile(path, content, 0o600); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
	return certFile, keyFile, caFile
}

func TestEtcdTLSConfig_Configured(t *testing.T) {
	tests := []struct {
		name string
		tls  EtcdTLSConfig
		want bool
	}{
		{"empty", EtcdTLSConfig{}, false},
		{"ca only", EtcdTLSConfig{CAFile: "ca.pem"}, true},
		{"cert only", EtcdTLSConfig{CertFile: "c.pem"}, true},
		{"key only", EtcdTLSConfig{KeyFile: "k.pem"}, true},
		{"insecure", EtcdTLSConfig{InsecureSkipVerify: true}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.tls.Configured(); got != tc.want {
				t.Errorf("Configured() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestClientTLS_NilWhenUnconfigured(t *testing.T) {
	cfg := &EtcdConfig{}
	got, err := cfg.ClientTLS()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil *tls.Config when TLS is unconfigured, got %+v", got)
	}
}

func TestClientTLS_WithCertKeyAndCA(t *testing.T) {
	certFile, keyFile, caFile := writeSelfSignedPEMs(t)
	cfg := &EtcdConfig{TLS: EtcdTLSConfig{CertFile: certFile, KeyFile: keyFile, CAFile: caFile}}

	got, err := cfg.ClientTLS()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil *tls.Config")
	}
	if len(got.Certificates) != 1 {
		t.Errorf("expected 1 client certificate, got %d", len(got.Certificates))
	}
	if got.RootCAs == nil {
		t.Error("expected RootCAs to be populated from the CA file")
	}
}

func TestClientTLS_InsecureSkipVerifyOnly(t *testing.T) {
	cfg := &EtcdConfig{TLS: EtcdTLSConfig{InsecureSkipVerify: true}}
	got, err := cfg.ClientTLS()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil || !got.InsecureSkipVerify {
		t.Errorf("expected InsecureSkipVerify=true in tls.Config, got %+v", got)
	}
}

func TestClientTLS_BadCAFile(t *testing.T) {
	t.Run("missing", func(t *testing.T) {
		cfg := &EtcdConfig{TLS: EtcdTLSConfig{CAFile: "/nonexistent/ca.pem"}}
		if _, err := cfg.ClientTLS(); err == nil {
			t.Error("expected error for missing CA file")
		}
	})
	t.Run("junk", func(t *testing.T) {
		dir := t.TempDir()
		ca := filepath.Join(dir, "ca.pem")
		if err := os.WriteFile(ca, []byte("not a pem"), 0o600); err != nil {
			t.Fatalf("write: %v", err)
		}
		cfg := &EtcdConfig{TLS: EtcdTLSConfig{CAFile: ca}}
		if _, err := cfg.ClientTLS(); err == nil {
			t.Error("expected error for CA file with no certificates")
		}
	})
}

func TestClientTLS_BadKeypair(t *testing.T) {
	cfg := &EtcdConfig{TLS: EtcdTLSConfig{CertFile: "/nonexistent/cert.pem", KeyFile: "/nonexistent/key.pem"}}
	if _, err := cfg.ClientTLS(); err == nil {
		t.Error("expected error for missing keypair files")
	}
}

func TestConfig_Validate_TLSCertWithoutKey(t *testing.T) {
	cfg := validConfig()
	cfg.Etcd.TLS.CertFile = "cert.pem"
	if err := cfg.validate(); err == nil {
		t.Error("expected error when cert_file is set without key_file")
	}
}

func TestConfig_Validate_TLSKeyWithoutCert(t *testing.T) {
	cfg := validConfig()
	cfg.Etcd.TLS.KeyFile = "key.pem"
	if err := cfg.validate(); err == nil {
		t.Error("expected error when key_file is set without cert_file")
	}
}

func TestConfig_Validate_UsernameWithoutPassword(t *testing.T) {
	cfg := validConfig()
	cfg.Etcd.Username = "admin"
	if err := cfg.validate(); err == nil {
		t.Error("expected error when username is set without password")
	}
}

func TestConfig_Validate_UsernameWithPassword(t *testing.T) {
	cfg := validConfig()
	cfg.Etcd.Username = "admin"
	cfg.Etcd.Password = "secret"
	if err := cfg.validate(); err != nil {
		t.Errorf("expected valid config with username+password, got: %v", err)
	}
}

func TestConfig_Validate_MetricsRequiresListenAddr(t *testing.T) {
	cfg := validConfig()
	cfg.Metrics.Enabled = true
	cfg.HTTP.ListenAddr = ""
	if err := cfg.validate(); err == nil {
		t.Error("expected error when metrics.enabled is true but http.listen_addr is empty")
	}

	cfg.HTTP.ListenAddr = ":8080"
	if err := cfg.validate(); err != nil {
		t.Errorf("expected valid config with metrics enabled and a listen addr, got: %v", err)
	}
}
