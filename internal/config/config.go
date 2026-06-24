package config

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
)

// Config is the top-level configuration struct.
type Config struct {
	App     AppConfig     `mapstructure:"app"`
	Etcd    EtcdConfig    `mapstructure:"etcd"`
	Logging LoggingConfig `mapstructure:"log"`
	HTTP    HTTPConfig    `mapstructure:"http"`
	Metrics MetricsConfig `mapstructure:"metrics"`
	Docker  DockerConfig  `mapstructure:"docker"`
}

// HeartbeatKeyPrefix is the etcd key prefix under which per-host liveness
// (heartbeat) keys are written. It lives deliberately outside etcd.path_prefix
// so CoreDNS never serves these keys and record listing never parses them;
// validate() enforces that the two prefixes do not overlap.
const HeartbeatKeyPrefix = "/docker-coredns-sync/heartbeat"

// DockerConfig configures the Docker event subscription, including the
// reconnect behavior when the event stream drops.
type DockerConfig struct {
	EventBufferSize         int     `mapstructure:"event_buffer_size"`
	ReconnectInitialBackoff float64 `mapstructure:"reconnect_initial_backoff"` // seconds
	ReconnectMaxBackoff     float64 `mapstructure:"reconnect_max_backoff"`     // seconds
}

// HTTPConfig configures the auxiliary HTTP server that serves the
// health/readiness and metrics endpoints.
type HTTPConfig struct {
	Enabled    bool   `mapstructure:"enabled"`
	ListenAddr string `mapstructure:"listen_addr"`
}

// MetricsConfig gates the Prometheus /metrics endpoint, which is served on the
// shared HTTP server (see HTTPConfig). When enabled, the HTTP server starts
// even if http.enabled is false.
type MetricsConfig struct {
	Enabled bool `mapstructure:"enabled"`
}

// AppConfig holds application-specific configuration.
type AppConfig struct {
	DockerLabelPrefix string `mapstructure:"docker_label_prefix"`
	HostIPv4          string `mapstructure:"host_ipv4"`
	HostIPv6          string `mapstructure:"host_ipv6"`
	Hostname          string `mapstructure:"hostname"`
	PollInterval      int    `mapstructure:"poll_interval"`
	// DryRun, when true, makes the reconciliation loop log the planned
	// changes without writing to or removing anything from etcd.
	DryRun bool `mapstructure:"dry_run"`
	// RecordTTL is the default DNS record TTL in seconds. Zero means "unset"
	// (the ttl field is omitted from the etcd value and CoreDNS applies its
	// own default). A per-record `coredns.<kind>[.<alias>].ttl` label overrides it.
	RecordTTL uint32 `mapstructure:"record_ttl"`
	// HeartbeatTTL is the lease TTL in seconds for this host's liveness key.
	// It doubles as the grace period before another host may garbage-collect
	// records owned by a host that has stopped renewing. Heartbeating is always
	// on; this value must be greater than zero.
	HeartbeatTTL int `mapstructure:"heartbeat_ttl"`
}

// EtcdConfig holds etcd-related configuration.
type EtcdConfig struct {
	Endpoints         []string      `mapstructure:"endpoints"`
	PathPrefix        string        `mapstructure:"path_prefix"`
	Username          string        `mapstructure:"username"`
	Password          string        `mapstructure:"password"`
	TLS               EtcdTLSConfig `mapstructure:"tls"`
	LockTTL           float64       `mapstructure:"lock_ttl"`
	LockTimeout       float64       `mapstructure:"lock_timeout"`
	LockRetryInterval float64       `mapstructure:"lock_retry_interval"`
}

// EtcdTLSConfig configures TLS for the etcd client connection. It is required
// for any non-loopback etcd deployment served over https://.
type EtcdTLSConfig struct {
	CAFile             string `mapstructure:"ca_file"`
	CertFile           string `mapstructure:"cert_file"`
	KeyFile            string `mapstructure:"key_file"`
	InsecureSkipVerify bool   `mapstructure:"insecure_skip_verify"`
}

// Configured reports whether any TLS setting is present, i.e. whether the etcd
// client should be built with a TLS configuration.
func (t EtcdTLSConfig) Configured() bool {
	return t.CAFile != "" || t.CertFile != "" || t.KeyFile != "" || t.InsecureSkipVerify
}

// hasHTTPSEndpoint reports whether any configured endpoint uses the https
// scheme.
func (c *EtcdConfig) hasHTTPSEndpoint() bool {
	for _, e := range c.Endpoints {
		if strings.HasPrefix(e, "https://") {
			return true
		}
	}
	return false
}

// UsesTLS reports whether the etcd connection will be encrypted: either TLS is
// explicitly configured, or at least one endpoint is https://.
func (c *EtcdConfig) UsesTLS() bool {
	return c.TLS.Configured() || c.hasHTTPSEndpoint()
}

// ClientTLS builds a *tls.Config for the etcd connection. When no TLS settings
// are present it returns (nil, nil) for plain http:// endpoints, but for an
// https:// endpoint it returns a config that verifies the server against the
// system root CAs — otherwise the client would silently dial without TLS and
// fail with an opaque transport error. A client cert/key pair and a CA root are
// loaded only when their respective files are set.
func (c *EtcdConfig) ClientTLS() (*tls.Config, error) {
	t := c.TLS
	if !t.Configured() {
		if c.hasHTTPSEndpoint() {
			return &tls.Config{MinVersion: tls.VersionTLS12}, nil
		}
		return nil, nil
	}
	cfg := &tls.Config{
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: t.InsecureSkipVerify, //nolint:gosec // opt-in via config for self-signed setups
	}
	if t.CertFile != "" || t.KeyFile != "" {
		cert, err := tls.LoadX509KeyPair(t.CertFile, t.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("load etcd client keypair: %w", err)
		}
		cfg.Certificates = []tls.Certificate{cert}
	}
	if t.CAFile != "" {
		pem, err := os.ReadFile(t.CAFile)
		if err != nil {
			return nil, fmt.Errorf("read etcd CA file: %w", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pem) {
			return nil, fmt.Errorf("no certificates found in etcd CA file %q", t.CAFile)
		}
		cfg.RootCAs = pool
	}
	return cfg, nil
}

// LoggingConfig holds the logging-related configuration.
type LoggingConfig struct {
	Level string `mapstructure:"level"`
}

// HTTPServerEnabled reports whether the auxiliary HTTP server should run, i.e.
// the health endpoints or the metrics endpoint (or both) are enabled. It is the
// single source of truth for this decision, used by both validation and startup.
func (c *Config) HTTPServerEnabled() bool {
	return c.HTTP.Enabled || c.Metrics.Enabled
}

// Load initializes, loads, and validates the config in one public call.
func Load() (*Config, error) {
	if err := initConfig(); err != nil {
		return nil, err
	}

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unable to decode into struct: %w", err)
	}

	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return &cfg, nil
}

func initConfig() error {
	// Respect the --config CLI flag if set
	if cfgFile := viper.GetString("config"); cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		// Default config file name
		viper.SetConfigName("config")
		viper.SetConfigType("yaml")

		// Add common config paths
		if configDir, err := os.UserConfigDir(); err == nil {
			viper.AddConfigPath(filepath.Join(configDir, "docker-coredns-sync"))
		}
		viper.AddConfigPath("/etc/docker-coredns-sync")
		viper.AddConfigPath("/config")
		viper.AddConfigPath(".")
	}

	// Environment variable support
	viper.SetEnvPrefix("DOCKER_COREDNS_SYNC")
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.AutomaticEnv()

	// Set Viper defaults
	viper.SetDefault("app.docker_label_prefix", "coredns")
	viper.SetDefault("app.host_ipv4", "")
	viper.SetDefault("app.host_ipv6", "")
	viper.SetDefault("app.hostname", "")
	viper.SetDefault("app.poll_interval", 5)
	viper.SetDefault("app.dry_run", false)
	viper.SetDefault("app.record_ttl", 0)
	viper.SetDefault("app.heartbeat_ttl", 30)
	viper.SetDefault("etcd.endpoints", []string{"http://localhost:2379"})
	viper.SetDefault("etcd.path_prefix", "/skydns")
	viper.SetDefault("etcd.lock_ttl", 5.0)
	viper.SetDefault("etcd.lock_timeout", 2.0)
	viper.SetDefault("etcd.lock_retry_interval", 0.1)
	viper.SetDefault("log.level", "INFO")
	viper.SetDefault("etcd.username", "")
	viper.SetDefault("etcd.password", "")
	viper.SetDefault("etcd.tls.ca_file", "")
	viper.SetDefault("etcd.tls.cert_file", "")
	viper.SetDefault("etcd.tls.key_file", "")
	viper.SetDefault("etcd.tls.insecure_skip_verify", false)
	viper.SetDefault("http.enabled", false)
	viper.SetDefault("http.listen_addr", ":8080")
	viper.SetDefault("metrics.enabled", false)
	viper.SetDefault("docker.event_buffer_size", 100)
	viper.SetDefault("docker.reconnect_initial_backoff", 1.0)
	viper.SetDefault("docker.reconnect_max_backoff", 30.0)

	// Read config file if it exists
	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return fmt.Errorf("error reading config file: %w", err)
		}
	}

	return nil
}

// validate checks for config consistency.
func (c *Config) validate() error {
	if c.App.DockerLabelPrefix == "" {
		return fmt.Errorf("app.docker_label_prefix cannot be empty")
	}
	if v := c.App.HostIPv4; v != "" && !isValidIPv4(v) {
		return fmt.Errorf("app.host_ipv4 must be a valid IPv4 address, got: %q", v)
	}
	if v := c.App.HostIPv6; v != "" && !isValidIPv6(v) {
		return fmt.Errorf("app.host_ipv6 must be a valid IPv6 address, got: %q", v)
	}
	if c.App.Hostname == "" {
		return fmt.Errorf("app.hostname cannot be empty")
	}
	if c.App.PollInterval <= 0 {
		return fmt.Errorf("app.poll_interval must be greater than 0")
	}
	if c.App.HeartbeatTTL <= 0 {
		return fmt.Errorf("app.heartbeat_ttl must be greater than 0")
	}
	if len(c.Etcd.Endpoints) == 0 {
		return fmt.Errorf("etcd.endpoints must have at least one endpoint")
	}
	for _, e := range c.Etcd.Endpoints {
		if !strings.HasPrefix(e, "http://") && !strings.HasPrefix(e, "https://") {
			return fmt.Errorf("invalid endpoint: %s", e)
		}
	}
	if c.Etcd.PathPrefix == "" {
		return fmt.Errorf("etcd.path_prefix cannot be empty")
	}
	// The heartbeat keys must not fall under path_prefix (or vice versa), or
	// CoreDNS would try to serve them and List() would parse them as DNS records.
	if pp := strings.TrimSuffix(c.Etcd.PathPrefix, "/"); pp == "" ||
		strings.HasPrefix(HeartbeatKeyPrefix+"/", pp+"/") ||
		strings.HasPrefix(pp+"/", HeartbeatKeyPrefix+"/") {
		return fmt.Errorf("etcd.path_prefix %q overlaps the reserved heartbeat key prefix %q; choose a non-overlapping prefix", c.Etcd.PathPrefix, HeartbeatKeyPrefix)
	}
	if (c.Etcd.TLS.CertFile == "") != (c.Etcd.TLS.KeyFile == "") {
		return fmt.Errorf("etcd.tls.cert_file and etcd.tls.key_file must be provided together")
	}
	if c.Etcd.TLS.InsecureSkipVerify && c.Etcd.TLS.CAFile != "" {
		return fmt.Errorf("etcd.tls.insecure_skip_verify cannot be combined with etcd.tls.ca_file: the CA would be ignored, giving a false sense of verification")
	}
	if c.Etcd.TLS.Configured() && !c.Etcd.hasHTTPSEndpoint() {
		return fmt.Errorf("etcd.tls.* is configured but no etcd.endpoints uses https://; the TLS settings would be silently ignored")
	}
	if c.Etcd.Username != "" && c.Etcd.Password == "" {
		return fmt.Errorf("etcd.password must be set when etcd.username is provided")
	}
	if c.Etcd.Password != "" && c.Etcd.Username == "" {
		return fmt.Errorf("etcd.username must be set when etcd.password is provided")
	}
	if c.Etcd.LockTTL <= 0 {
		return fmt.Errorf("etcd.lock_ttl must be > 0")
	}
	if c.Etcd.LockTimeout <= 0 {
		return fmt.Errorf("etcd.lock_timeout must be > 0")
	}
	if c.Etcd.LockRetryInterval <= 0 {
		return fmt.Errorf("etcd.lock_retry_interval must be > 0")
	}
	validLevels := map[string]struct{}{
		"TRACE": {}, "DEBUG": {}, "INFO": {}, "WARN": {}, "ERROR": {}, "FATAL": {},
	}
	if _, ok := validLevels[strings.ToUpper(c.Logging.Level)]; !ok {
		return fmt.Errorf("log.level must be a valid log level, got: %s", c.Logging.Level)
	}
	if c.HTTPServerEnabled() && strings.TrimSpace(c.HTTP.ListenAddr) == "" {
		return fmt.Errorf("http.listen_addr cannot be empty when http.enabled or metrics.enabled is true")
	}
	if c.Docker.EventBufferSize <= 0 {
		return fmt.Errorf("docker.event_buffer_size must be greater than 0")
	}
	if c.Docker.ReconnectInitialBackoff <= 0 {
		return fmt.Errorf("docker.reconnect_initial_backoff must be greater than 0")
	}
	if c.Docker.ReconnectMaxBackoff < c.Docker.ReconnectInitialBackoff {
		return fmt.Errorf("docker.reconnect_max_backoff must be >= docker.reconnect_initial_backoff")
	}
	return nil
}

func isValidIPv4(s string) bool {
	ip := net.ParseIP(strings.TrimSpace(s))
	return ip != nil && ip.To4() != nil
}

func isValidIPv6(s string) bool {
	ip := net.ParseIP(strings.TrimSpace(s))
	// true IPv6: has a 16-byte form and is not IPv4-mapped (To4()==nil)
	return ip != nil && ip.To16() != nil && ip.To4() == nil
}
