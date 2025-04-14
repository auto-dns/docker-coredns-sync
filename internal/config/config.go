package config

import (
	"fmt"
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

// InitConfig performs the initial configuration: setting defaults, specifying the config file, and reading it.
func InitConfig() error {
	// Set defaults for each sub-configuration.
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

	// Map env vars to config fields
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

	// Specify the config file details.
	viper.SetConfigName("config") // Looks for config.yaml
	viper.SetConfigType("yaml")
	viper.AddConfigPath(".") // current directory

	// Read the config file if available.
	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return fmt.Errorf("error reading config file: %w", err)
		}
		// If the file is not found, just continue with defaults and env vars.
	}

	// Enable automatic environment variable binding.
	viper.AutomaticEnv()
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	return nil
}

// Load unmarshals the configuration into the Config struct.
func Load() (*Config, error) {
	var config Config
	if err := viper.Unmarshal(&config); err != nil {
		return nil, fmt.Errorf("unable to decode into struct: %w", err)
	}
	return &config, nil
}
