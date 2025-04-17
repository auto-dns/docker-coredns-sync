package config

import (
	"fmt"
	"net"
	"strings"

	"github.com/spf13/viper"
)

// AppConfig holds application-specific configuration.
type AppConfig struct {
	AllowedRecordTypes []string `mapstructure:"allowed_record_types"`
	DockerLabelPrefix  string   `mapstructure:"docker_label_prefix"`
	HostIP             string   `mapstructure:"host_ip"`
	Hostname           string   `mapstructure:"hostname"`
	PollInterval       int      `mapstructure:"poll_interval"`
}

// LoggingConfig holds the logging-related configuration.
type LoggingConfig struct {
	Level string `mapstructure:"level"`
}

// EtcdConfig holds etcd-related configuration.
type EtcdConfig struct {
	Host              string  `mapstructure:"host"`
	Port              int     `mapstructure:"port"`
	PathPrefix        string  `mapstructure:"path_prefix"`
	LockTTL           float64 `mapstructure:"lock_ttl"`
	LockTimeout       float64 `mapstructure:"lock_timeout"`
	LockRetryInterval float64 `mapstructure:"lock_retry_interval"`
}

// Config is the top-level configuration struct.
type Config struct {
	App     AppConfig     `mapstructure:"app"`
	Logging LoggingConfig `mapstructure:"log"`
	Etcd    EtcdConfig    `mapstructure:"etcd"`
}

// Load initializes, loads, normalizes, and validates the config in one public call.
func Load() (*Config, error) {
	if err := initConfig(); err != nil {
		return nil, err
	}

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unable to decode into struct: %w", err)
	}

	cfg.normalize()

	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return &cfg, nil
}

// initConfig sets defaults and reads the config file/environment variables.
func initConfig() error {
	viper.SetDefault("app.allowed_record_types", []string{"A", "CNAME"})
	viper.SetDefault("app.docker_label_prefix", "coredns")
	viper.SetDefault("app.host_ip", "127.0.0.1")
	viper.SetDefault("app.hostname", "your-hostname")
	viper.SetDefault("app.poll_interval", 5)
	viper.SetDefault("log.level", "INFO")
	viper.SetDefault("etcd.host", "localhost")
	viper.SetDefault("etcd.port", 2379)
	viper.SetDefault("etcd.path_prefix", "/skydns")
	viper.SetDefault("etcd.lock_ttl", 5.0)
	viper.SetDefault("etcd.lock_timeout", 2.0)
	viper.SetDefault("etcd.lock_retry_interval", 0.1)

	viper.BindEnv("app.docker_label_prefix", "DOCKER_LABEL_PREFIX")
	viper.BindEnv("app.host_ip", "HOST_IP")
	viper.BindEnv("app.hostname", "HOSTNAME")
	viper.BindEnv("app.poll_interval", "POLL_INTERVAL")
	viper.BindEnv("log.level", "LOG_LEVEL")
	viper.BindEnv("etcd.host", "ETCD_HOST")
	viper.BindEnv("etcd.port", "ETCD_PORT")
	viper.BindEnv("etcd.path_prefix", "ETCD_PATH_PREFIX")
	viper.BindEnv("etcd.lock_ttl", "ETCD_LOCK_TTL")
	viper.BindEnv("etcd.lock_timeout", "ETCD_LOCK_TIMEOUT")
	viper.BindEnv("etcd.lock_retry_interval", "ETCD_LOCK_RETRY_INTERVAL")

	viper.SetConfigName("config") // config.yaml
	viper.SetConfigType("yaml")
	viper.AddConfigPath(".")

	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return fmt.Errorf("error reading config file: %w", err)
		}
	}

	viper.AutomaticEnv()
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	return nil
}

// normalize adjusts config values to standard forms.
func (c *Config) normalize() {
	for i, rt := range c.App.AllowedRecordTypes {
		c.App.AllowedRecordTypes[i] = strings.ToUpper(rt)
	}
}

// validate checks for config consistency.
func (c *Config) validate() error {
	if c.App.DockerLabelPrefix == "" {
		return fmt.Errorf("app.docker_label_prefix cannot be empty")
	}
	if len(c.App.AllowedRecordTypes) == 0 {
		return fmt.Errorf("app.allowed_record_types must have at least one entry")
	}
	validTypes := map[string]struct{}{"A": {}, "CNAME": {}}
	for _, t := range c.App.AllowedRecordTypes {
		if _, ok := validTypes[t]; !ok {
			return fmt.Errorf("unsupported record type in app.allowed_record_types: %s", t)
		}
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
	if c.Etcd.Host == "" {
		return fmt.Errorf("etcd.host cannot be empty")
	}
	if c.Etcd.Port <= 0 || c.Etcd.Port > 65535 {
		return fmt.Errorf("etcd.port must be a valid TCP port")
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
