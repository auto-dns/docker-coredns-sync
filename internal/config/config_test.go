package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
)

func validConfig() *Config {
	return &Config{
		App: AppConfig{
			DockerLabelPrefix: "coredns",
			HostIPv4:          "192.168.1.1",
			HostIPv6:          "::1",
			Hostname:          "test-host",
			PollInterval:      5,
		},
		Etcd: EtcdConfig{
			Endpoints:         []string{"http://localhost:2379"},
			PathPrefix:        "/skydns",
			LockTTL:           5.0,
			LockTimeout:       2.0,
			LockRetryInterval: 0.1,
		},
		Logging: LoggingConfig{
			Level: "INFO",
		},
	}
}

func TestConfig_Validate_ValidConfig(t *testing.T) {
	cfg := validConfig()

	err := cfg.validate()

	if err != nil {
		t.Errorf("expected valid config to pass validation, got: %v", err)
	}
}

func TestConfig_Validate_EmptyLabelPrefix(t *testing.T) {
	cfg := validConfig()
	cfg.App.DockerLabelPrefix = ""

	err := cfg.validate()

	if err == nil {
		t.Error("expected error for empty DockerLabelPrefix")
	}
}

func TestConfig_Validate_InvalidIPv4(t *testing.T) {
	tests := []string{
		"not-an-ip",
		"256.256.256.256",
		"::1", // IPv6 as IPv4
		"192.168.1",
	}

	for _, ip := range tests {
		t.Run(ip, func(t *testing.T) {
			cfg := validConfig()
			cfg.App.HostIPv4 = ip

			err := cfg.validate()

			if err == nil {
				t.Errorf("expected error for invalid IPv4 %q", ip)
			}
		})
	}
}

func TestConfig_Validate_EmptyIPv4Allowed(t *testing.T) {
	cfg := validConfig()
	cfg.App.HostIPv4 = ""

	err := cfg.validate()

	if err != nil {
		t.Errorf("expected empty IPv4 to be allowed, got: %v", err)
	}
}

func TestConfig_Validate_InvalidIPv6(t *testing.T) {
	tests := []string{
		"not-an-ip",
		"192.168.1.1", // IPv4 as IPv6
		"gggg::1",
	}

	for _, ip := range tests {
		t.Run(ip, func(t *testing.T) {
			cfg := validConfig()
			cfg.App.HostIPv6 = ip

			err := cfg.validate()

			if err == nil {
				t.Errorf("expected error for invalid IPv6 %q", ip)
			}
		})
	}
}

func TestConfig_Validate_EmptyIPv6Allowed(t *testing.T) {
	cfg := validConfig()
	cfg.App.HostIPv6 = ""

	err := cfg.validate()

	if err != nil {
		t.Errorf("expected empty IPv6 to be allowed, got: %v", err)
	}
}

func TestConfig_Validate_EmptyHostname(t *testing.T) {
	cfg := validConfig()
	cfg.App.Hostname = ""

	err := cfg.validate()

	if err == nil {
		t.Error("expected error for empty Hostname")
	}
}

func TestConfig_Validate_ZeroPollInterval(t *testing.T) {
	cfg := validConfig()
	cfg.App.PollInterval = 0

	err := cfg.validate()

	if err == nil {
		t.Error("expected error for zero PollInterval")
	}
}

func TestConfig_Validate_NegativePollInterval(t *testing.T) {
	cfg := validConfig()
	cfg.App.PollInterval = -5

	err := cfg.validate()

	if err == nil {
		t.Error("expected error for negative PollInterval")
	}
}

func TestConfig_Validate_NoEndpoints(t *testing.T) {
	cfg := validConfig()
	cfg.Etcd.Endpoints = []string{}

	err := cfg.validate()

	if err == nil {
		t.Error("expected error for empty Endpoints")
	}
}

func TestConfig_Validate_InvalidEndpoint(t *testing.T) {
	tests := []string{
		"localhost:2379",       // no scheme
		"ftp://localhost:2379", // wrong scheme
		"tcp://localhost:2379", // wrong scheme
	}

	for _, endpoint := range tests {
		t.Run(endpoint, func(t *testing.T) {
			cfg := validConfig()
			cfg.Etcd.Endpoints = []string{endpoint}

			err := cfg.validate()

			if err == nil {
				t.Errorf("expected error for invalid endpoint %q", endpoint)
			}
		})
	}
}

func TestConfig_Validate_ValidEndpoints(t *testing.T) {
	tests := []string{
		"http://localhost:2379",
		"https://localhost:2379",
		"http://192.168.1.1:2379",
		"https://etcd.example.com:2379",
	}

	for _, endpoint := range tests {
		t.Run(endpoint, func(t *testing.T) {
			cfg := validConfig()
			cfg.Etcd.Endpoints = []string{endpoint}

			err := cfg.validate()

			if err != nil {
				t.Errorf("expected valid endpoint %q to pass, got: %v", endpoint, err)
			}
		})
	}
}

func TestConfig_Validate_MultipleEndpoints(t *testing.T) {
	cfg := validConfig()
	cfg.Etcd.Endpoints = []string{
		"http://localhost:2379",
		"http://localhost:2380",
		"http://localhost:2381",
	}

	err := cfg.validate()

	if err != nil {
		t.Errorf("expected multiple valid endpoints to pass, got: %v", err)
	}
}

func TestConfig_Validate_EmptyPathPrefix(t *testing.T) {
	cfg := validConfig()
	cfg.Etcd.PathPrefix = ""

	err := cfg.validate()

	if err == nil {
		t.Error("expected error for empty PathPrefix")
	}
}

func TestConfig_Validate_InvalidLockTTL(t *testing.T) {
	tests := []float64{0, -1, -5.0}

	for _, ttl := range tests {
		t.Run("ttl", func(t *testing.T) {
			cfg := validConfig()
			cfg.Etcd.LockTTL = ttl

			err := cfg.validate()

			if err == nil {
				t.Errorf("expected error for LockTTL %v", ttl)
			}
		})
	}
}

func TestConfig_Validate_InvalidLockTimeout(t *testing.T) {
	tests := []float64{0, -1, -5.0}

	for _, timeout := range tests {
		t.Run("timeout", func(t *testing.T) {
			cfg := validConfig()
			cfg.Etcd.LockTimeout = timeout

			err := cfg.validate()

			if err == nil {
				t.Errorf("expected error for LockTimeout %v", timeout)
			}
		})
	}
}

func TestConfig_Validate_InvalidLockRetryInterval(t *testing.T) {
	tests := []float64{0, -1, -0.5}

	for _, interval := range tests {
		t.Run("interval", func(t *testing.T) {
			cfg := validConfig()
			cfg.Etcd.LockRetryInterval = interval

			err := cfg.validate()

			if err == nil {
				t.Errorf("expected error for LockRetryInterval %v", interval)
			}
		})
	}
}

func TestConfig_Validate_InvalidLogLevel(t *testing.T) {
	tests := []string{
		"VERBOSE",
		"WARNING",
		"CRITICAL",
		"",
		"info ", // trailing space
	}

	for _, level := range tests {
		t.Run(level, func(t *testing.T) {
			cfg := validConfig()
			cfg.Logging.Level = level

			err := cfg.validate()

			if err == nil {
				t.Errorf("expected error for invalid log level %q", level)
			}
		})
	}
}

func TestConfig_Validate_ValidLogLevels(t *testing.T) {
	tests := []string{"TRACE", "DEBUG", "INFO", "WARN", "ERROR", "FATAL", "trace", "debug", "info", "warn", "error", "fatal"}

	for _, level := range tests {
		t.Run(level, func(t *testing.T) {
			cfg := validConfig()
			cfg.Logging.Level = level

			err := cfg.validate()

			if err != nil {
				t.Errorf("expected valid log level %q to pass, got: %v", level, err)
			}
		})
	}
}

func TestIsValidIPv4_ValidAddresses(t *testing.T) {
	valid := []string{
		"192.168.1.1",
		"10.0.0.1",
		"172.16.0.1",
		"8.8.8.8",
		"127.0.0.1",
		"0.0.0.0",
		"255.255.255.255",
	}

	for _, ip := range valid {
		t.Run(ip, func(t *testing.T) {
			if !isValidIPv4(ip) {
				t.Errorf("expected %q to be valid IPv4", ip)
			}
		})
	}
}

func TestIsValidIPv4_InvalidAddresses(t *testing.T) {
	invalid := []string{
		"",
		"not-an-ip",
		"::1",
		"2001:db8::1",
		"192.168.1",
		"192.168.1.1.1",
		"256.0.0.1",
	}

	for _, ip := range invalid {
		t.Run(ip, func(t *testing.T) {
			if isValidIPv4(ip) {
				t.Errorf("expected %q to be invalid IPv4", ip)
			}
		})
	}
}

func TestIsValidIPv6_ValidAddresses(t *testing.T) {
	valid := []string{
		"::1",
		"2001:db8::1",
		"fe80::1",
		"2001:0db8:85a3:0000:0000:8a2e:0370:7334",
	}

	for _, ip := range valid {
		t.Run(ip, func(t *testing.T) {
			if !isValidIPv6(ip) {
				t.Errorf("expected %q to be valid IPv6", ip)
			}
		})
	}
}

func TestIsValidIPv6_InvalidAddresses(t *testing.T) {
	invalid := []string{
		"",
		"not-an-ip",
		"192.168.1.1",
		"10.0.0.1",
	}

	for _, ip := range invalid {
		t.Run(ip, func(t *testing.T) {
			if isValidIPv6(ip) {
				t.Errorf("expected %q to be invalid IPv6", ip)
			}
		})
	}
}

func TestIsValidIPv4_WhitespaceHandling(t *testing.T) {
	// Function trims whitespace
	if !isValidIPv4("  192.168.1.1  ") {
		t.Error("expected whitespace-padded IPv4 to be valid after trim")
	}
}

func TestIsValidIPv6_WhitespaceHandling(t *testing.T) {
	// Function trims whitespace
	if !isValidIPv6("  ::1  ") {
		t.Error("expected whitespace-padded IPv6 to be valid after trim")
	}
}

func resetViper() {
	viper.Reset()
	viper.SetEnvPrefix("DOCKER_COREDNS_SYNC")
}

func TestLoad_Success_FromEnvVars(t *testing.T) {
	resetViper()
	defer resetViper()

	t.Setenv("DOCKER_COREDNS_SYNC_APP_DOCKER_LABEL_PREFIX", "coredns")
	t.Setenv("DOCKER_COREDNS_SYNC_APP_HOSTNAME", "test-host")
	t.Setenv("DOCKER_COREDNS_SYNC_APP_HOST_IPV4", "192.168.1.1")
	t.Setenv("DOCKER_COREDNS_SYNC_APP_HOST_IPV6", "::1")
	t.Setenv("DOCKER_COREDNS_SYNC_APP_POLL_INTERVAL", "10")
	t.Setenv("DOCKER_COREDNS_SYNC_ETCD_ENDPOINTS", "http://localhost:2379")
	t.Setenv("DOCKER_COREDNS_SYNC_ETCD_PATH_PREFIX", "/skydns")
	t.Setenv("DOCKER_COREDNS_SYNC_ETCD_LOCK_TTL", "5.0")
	t.Setenv("DOCKER_COREDNS_SYNC_ETCD_LOCK_TIMEOUT", "2.0")
	t.Setenv("DOCKER_COREDNS_SYNC_ETCD_LOCK_RETRY_INTERVAL", "0.1")
	t.Setenv("DOCKER_COREDNS_SYNC_LOG_LEVEL", "INFO")

	cfg, err := Load()

	if err != nil {
		t.Fatalf("expected Load to succeed, got error: %v", err)
	}
	if cfg.App.Hostname != "test-host" {
		t.Errorf("expected hostname 'test-host', got %q", cfg.App.Hostname)
	}
	if cfg.App.DockerLabelPrefix != "coredns" {
		t.Errorf("expected label prefix 'coredns', got %q", cfg.App.DockerLabelPrefix)
	}
}

func TestLoad_Success_FromConfigFile(t *testing.T) {
	resetViper()
	defer resetViper()

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	configContent := `
app:
  docker_label_prefix: "coredns"
  hostname: "file-host"
  host_ipv4: "10.0.0.1"
  host_ipv6: "fe80::1"
  poll_interval: 15
etcd:
  endpoints:
    - "http://etcd1:2379"
  path_prefix: "/dns"
  lock_ttl: 10.0
  lock_timeout: 5.0
  lock_retry_interval: 0.5
log:
  level: "DEBUG"
`
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	viper.Set("config", configPath)

	cfg, err := Load()

	if err != nil {
		t.Fatalf("expected Load to succeed, got error: %v", err)
	}
	if cfg.App.Hostname != "file-host" {
		t.Errorf("expected hostname 'file-host', got %q", cfg.App.Hostname)
	}
	if cfg.Etcd.PathPrefix != "/dns" {
		t.Errorf("expected path prefix '/dns', got %q", cfg.Etcd.PathPrefix)
	}
}

func TestLoad_Success_NoConfigFile_UsesDefaults(t *testing.T) {
	resetViper()
	defer resetViper()

	tmpDir := t.TempDir()
	oldWd, _ := os.Getwd()
	defer os.Chdir(oldWd)
	os.Chdir(tmpDir)

	t.Setenv("DOCKER_COREDNS_SYNC_APP_HOSTNAME", "default-host")

	cfg, err := Load()

	if err != nil {
		t.Fatalf("expected Load to succeed with defaults, got error: %v", err)
	}
	if cfg.App.DockerLabelPrefix != "coredns" {
		t.Errorf("expected default label prefix 'coredns', got %q", cfg.App.DockerLabelPrefix)
	}
	if cfg.App.PollInterval != 5 {
		t.Errorf("expected default poll interval 5, got %d", cfg.App.PollInterval)
	}
	if cfg.App.RecordTTL != 0 {
		t.Errorf("expected default record_ttl 0, got %d", cfg.App.RecordTTL)
	}
	if cfg.App.HeartbeatTTL != 30 {
		t.Errorf("expected default heartbeat_ttl 30, got %d", cfg.App.HeartbeatTTL)
	}
}

func TestLoad_InitConfigError_InvalidYAML(t *testing.T) {
	resetViper()
	defer resetViper()

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	invalidYAML := `
app:
  hostname: "test
  invalid yaml content [[[
`
	if err := os.WriteFile(configPath, []byte(invalidYAML), 0644); err != nil {
		t.Fatalf("failed to write invalid config file: %v", err)
	}

	viper.Set("config", configPath)

	_, err := Load()

	if err == nil {
		t.Error("expected Load to fail with invalid YAML")
	}
}

func TestLoad_ValidationError(t *testing.T) {
	resetViper()
	defer resetViper()

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	configContent := `
app:
  docker_label_prefix: ""
  hostname: "test-host"
  poll_interval: 5
etcd:
  endpoints:
    - "http://localhost:2379"
  path_prefix: "/skydns"
  lock_ttl: 5.0
  lock_timeout: 2.0
  lock_retry_interval: 0.1
log:
  level: "INFO"
`
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	viper.Set("config", configPath)

	_, err := Load()

	if err == nil {
		t.Error("expected Load to fail with validation error for empty label prefix")
	}
}

func TestLoad_WithConfigFlag(t *testing.T) {
	resetViper()
	defer resetViper()

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "custom-config.yaml")
	configContent := `
app:
  docker_label_prefix: "custom"
  hostname: "custom-host"
  poll_interval: 20
etcd:
  endpoints:
    - "http://custom-etcd:2379"
  path_prefix: "/custom"
  lock_ttl: 5.0
  lock_timeout: 2.0
  lock_retry_interval: 0.1
log:
  level: "WARN"
`
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	viper.Set("config", configPath)

	cfg, err := Load()

	if err != nil {
		t.Fatalf("expected Load to succeed, got error: %v", err)
	}
	if cfg.App.DockerLabelPrefix != "custom" {
		t.Errorf("expected label prefix 'custom', got %q", cfg.App.DockerLabelPrefix)
	}
	if cfg.App.Hostname != "custom-host" {
		t.Errorf("expected hostname 'custom-host', got %q", cfg.App.Hostname)
	}
}

func TestLoad_DefaultConfigPaths(t *testing.T) {
	resetViper()
	defer resetViper()

	tmpDir := t.TempDir()
	oldWd, _ := os.Getwd()
	defer os.Chdir(oldWd)
	os.Chdir(tmpDir)

	configPath := filepath.Join(tmpDir, "config.yaml")
	configContent := `
app:
  docker_label_prefix: "found"
  hostname: "found-host"
  poll_interval: 5
etcd:
  endpoints:
    - "http://localhost:2379"
  path_prefix: "/skydns"
  lock_ttl: 5.0
  lock_timeout: 2.0
  lock_retry_interval: 0.1
log:
  level: "INFO"
`
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	cfg, err := Load()

	if err != nil {
		t.Fatalf("expected Load to succeed, got error: %v", err)
	}
	if cfg.App.DockerLabelPrefix != "found" {
		t.Errorf("expected label prefix 'found', got %q", cfg.App.DockerLabelPrefix)
	}
}

func TestLoad_UnmarshalError(t *testing.T) {
	resetViper()
	defer resetViper()

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	configContent := `
app:
  docker_label_prefix: "test"
  hostname: "test-host"
  poll_interval: "not-an-int"
etcd:
  endpoints:
    - "http://localhost:2379"
  path_prefix: "/skydns"
  lock_ttl: 5.0
  lock_timeout: 2.0
  lock_retry_interval: 0.1
log:
  level: "INFO"
`
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	viper.Set("config", configPath)

	_, err := Load()

	if err == nil {
		t.Error("expected Load to fail with unmarshal error for string in int field")
	}
}
