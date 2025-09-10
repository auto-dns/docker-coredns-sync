package config

import (
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
}

// AppConfig holds application-specific configuration.
type AppConfig struct {
	RecordTypes       RecordTypesConfig `mapstructure:"record_types"`
	DockerLabelPrefix string            `mapstructure:"docker_label_prefix"`
	HostIP            string            `mapstructure:"host_ip"`
	Hostname          string            `mapstructure:"hostname"`
	PollInterval      int               `mapstructure:"poll_interval"`
}

type RecordTypesConfig struct {
	A     RecordTypeConfig `mapstructure:"a"`
	AAAA  RecordTypeConfig `mapstructure:"aaaa"`
	CNAME RecordTypeConfig `mapstructure:"cname"`
}

type RecordTypeConfig struct {
	Enabled bool `mapstructure:"enabled"`
}

// EtcdConfig holds etcd-related configuration.
type EtcdConfig struct {
	Endpoints         []string `mapstructure:"endpoints"`
	PathPrefix        string   `mapstructure:"path_prefix"`
	LockTTL           float64  `mapstructure:"lock_ttl"`
	LockTimeout       float64  `mapstructure:"lock_timeout"`
	LockRetryInterval float64  `mapstructure:"lock_retry_interval"`
}

// LoggingConfig holds the logging-related configuration.
type LoggingConfig struct {
	Level string `mapstructure:"level"`
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
	viper.SetDefault("app.allowed_record_types", []string{"A", "CNAME"})
	viper.SetDefault("app.docker_label_prefix", "coredns")
	viper.SetDefault("app.host_ip", "127.0.0.1")
	viper.SetDefault("app.hostname", "")
	viper.SetDefault("app.poll_interval", 5)
	viper.SetDefault("etcd.endpoints", []string{"http://localhost:2379"})
	viper.SetDefault("etcd.path_prefix", "/skydns")
	viper.SetDefault("etcd.lock_ttl", 5.0)
	viper.SetDefault("etcd.lock_timeout", 2.0)
	viper.SetDefault("etcd.lock_retry_interval", 0.1)
	viper.SetDefault("log.level", "INFO")

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

	if !c.App.RecordTypes.A.Enabled && !c.App.RecordTypes.AAAA.Enabled && !c.App.RecordTypes.CNAME.Enabled {
		return fmt.Errorf("app.record_types must have at least one record type enabled")
	}

	if c.App.DockerLabelPrefix == "" {
		return fmt.Errorf("app.docker_label_prefix cannot be empty")
	}
	if net.ParseIP(c.App.HostIP) == nil {
		return fmt.Errorf("app.host_ip must be a valid IP address")
	}
	if c.App.Hostname == "" {
		return fmt.Errorf("app.hostname cannot be empty")
	}
	if c.App.PollInterval <= 0 {
		return fmt.Errorf("app.poll_interval must be greater than 0")
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
	return nil
}
