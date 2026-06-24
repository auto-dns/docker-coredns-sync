package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/rs/zerolog"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/auto-dns/docker-coredns-sync/internal/app"
	"github.com/auto-dns/docker-coredns-sync/internal/config"
	"github.com/auto-dns/docker-coredns-sync/internal/logger"
)

type contextKey string

const configKey = contextKey("config")

type AppRunner interface {
	Run(ctx context.Context) error
	Close() error
}

type AppFactory func(cfg *config.Config, log zerolog.Logger) (AppRunner, error)

var defaultAppFactory AppFactory = func(cfg *config.Config, log zerolog.Logger) (AppRunner, error) {
	return app.New(cfg, log)
}

func runWithDeps(cfg *config.Config, factory AppFactory, sigCh <-chan os.Signal) error {
	logInstance := logger.SetupLogger(&cfg.Logging)

	application, err := factory(cfg, logInstance)
	if err != nil {
		return fmt.Errorf("failed to create app: %w", err)
	}
	defer func() {
		if cerr := application.Close(); cerr != nil {
			logInstance.Error().Err(cerr).Msg("error during app close")
		}
	}()

	ctx, stop := context.WithCancel(context.Background())
	defer stop()

	go func() {
		sig, ok := <-sigCh
		if ok {
			logInstance.Info().Msgf("Received signal: %v", sig)
			stop()
		}
	}()

	if err := application.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		return fmt.Errorf("app run error: %w", err)
	}
	// A context.Canceled error is the expected outcome of a signal-driven
	// shutdown, so it is treated as a clean exit (return nil).
	return nil
}

var rootCmd = &cobra.Command{
	Use:   "docker-coredns-sync",
	Short: "Synchronize Docker and CoreDNS via etcd",
	Long:  "A tool to synchronize container events with DNS records using etcd as a backend.",
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		ctx := context.WithValue(cmd.Context(), configKey, cfg)
		cmd.SetContext(ctx)
		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg := cmd.Context().Value(configKey).(*config.Config)

		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

		return runWithDeps(cfg, defaultAppFactory, sigCh)
	},
}

func init() {
	// Persistent config file override
	rootCmd.PersistentFlags().String("config", "", "Path to config file (e.g. ./config.yaml)")
	viper.BindPFlag("config", rootCmd.PersistentFlags().Lookup("config"))

	// AppConfig Flags
	rootCmd.PersistentFlags().String("app.docker-label-prefix", "", "Prefix used for Docker labels (e.g., 'coredns')")
	viper.BindPFlag("app.docker_label_prefix", rootCmd.PersistentFlags().Lookup("app.docker-label-prefix"))

	rootCmd.PersistentFlags().String("app.host-ipv4", "", "Host IPv4 address to use in A records")
	viper.BindPFlag("app.host_ipv4", rootCmd.PersistentFlags().Lookup("app.host-ipv4"))

	rootCmd.PersistentFlags().String("app.host-ipv6", "", "Host IPv6 address to use in AAAA records")
	viper.BindPFlag("app.host_ipv6", rootCmd.PersistentFlags().Lookup("app.host-ipv6"))

	rootCmd.PersistentFlags().String("app.hostname", "", "Logical hostname of this instance")
	viper.BindPFlag("app.hostname", rootCmd.PersistentFlags().Lookup("app.hostname"))

	rootCmd.PersistentFlags().Int("app.poll-interval", 0, "Polling interval (in seconds) for reconciliation")
	viper.BindPFlag("app.poll_interval", rootCmd.PersistentFlags().Lookup("app.poll-interval"))

	rootCmd.PersistentFlags().Bool("app.dry-run", false, "Log planned etcd changes without applying them")
	viper.BindPFlag("app.dry_run", rootCmd.PersistentFlags().Lookup("app.dry-run"))

	// Flag defaults are left at zero values so an unset flag does not override the
	// real defaults configured via viper.SetDefault in internal/config.
	rootCmd.PersistentFlags().Uint32("app.record-ttl", 0, "Default DNS record TTL in seconds (0 = unset; CoreDNS uses its own default)")
	viper.BindPFlag("app.record_ttl", rootCmd.PersistentFlags().Lookup("app.record-ttl"))

	rootCmd.PersistentFlags().Int("app.heartbeat-ttl", 0, "Lease TTL (seconds) for this host's liveness key; also the grace period before peers GC a stopped host's records")
	viper.BindPFlag("app.heartbeat_ttl", rootCmd.PersistentFlags().Lookup("app.heartbeat-ttl"))

	// EtcdConfig Flags
	rootCmd.PersistentFlags().StringArray("etcd-endpoints", []string{"http://localhost:2379"}, "etcd endpoints to connect to (can specify multiple times)")
	viper.BindPFlag("etcd.endpoints", rootCmd.PersistentFlags().Lookup("etcd-endpoints"))

	rootCmd.PersistentFlags().String("etcd.path-prefix", "", "etcd key path prefix (e.g., /skydns)")
	viper.BindPFlag("etcd.path_prefix", rootCmd.PersistentFlags().Lookup("etcd.path-prefix"))

	rootCmd.PersistentFlags().String("etcd.username", "", "Username for etcd authentication")
	viper.BindPFlag("etcd.username", rootCmd.PersistentFlags().Lookup("etcd.username"))

	// Note: there is intentionally no --etcd.password flag. A password on the
	// command line is exposed in the process list and shell history; set it via
	// the DOCKER_COREDNS_SYNC_ETCD_PASSWORD env var or the config file instead.

	rootCmd.PersistentFlags().String("etcd.tls.ca-file", "", "Path to the CA certificate for the etcd connection")
	viper.BindPFlag("etcd.tls.ca_file", rootCmd.PersistentFlags().Lookup("etcd.tls.ca-file"))

	rootCmd.PersistentFlags().String("etcd.tls.cert-file", "", "Path to the client certificate for the etcd connection")
	viper.BindPFlag("etcd.tls.cert_file", rootCmd.PersistentFlags().Lookup("etcd.tls.cert-file"))

	rootCmd.PersistentFlags().String("etcd.tls.key-file", "", "Path to the client key for the etcd connection")
	viper.BindPFlag("etcd.tls.key_file", rootCmd.PersistentFlags().Lookup("etcd.tls.key-file"))

	rootCmd.PersistentFlags().Bool("etcd.tls.insecure-skip-verify", false, "Skip verification of the etcd server certificate (insecure)")
	viper.BindPFlag("etcd.tls.insecure_skip_verify", rootCmd.PersistentFlags().Lookup("etcd.tls.insecure-skip-verify"))

	rootCmd.PersistentFlags().Float64("etcd.lock-ttl", 0, "TTL (in seconds) for etcd locks")
	viper.BindPFlag("etcd.lock_ttl", rootCmd.PersistentFlags().Lookup("etcd.lock-ttl"))

	rootCmd.PersistentFlags().Float64("etcd.lock-timeout", 0, "Timeout (in seconds) for acquiring etcd locks")
	viper.BindPFlag("etcd.lock_timeout", rootCmd.PersistentFlags().Lookup("etcd.lock-timeout"))

	rootCmd.PersistentFlags().Float64("etcd.lock-retry-interval", 0, "Interval (in seconds) to retry etcd lock acquisition")
	viper.BindPFlag("etcd.lock_retry_interval", rootCmd.PersistentFlags().Lookup("etcd.lock-retry-interval"))

	// LoggingConfig Flag
	rootCmd.PersistentFlags().String("log.level", "", "Log level (e.g., TRACE, DEBUG, INFO, WARN, ERROR, FATAL)")
	viper.BindPFlag("log.level", rootCmd.PersistentFlags().Lookup("log.level"))

	// HTTPConfig Flags
	rootCmd.PersistentFlags().Bool("http.enabled", false, "Enable the HTTP server for health/readiness endpoints")
	viper.BindPFlag("http.enabled", rootCmd.PersistentFlags().Lookup("http.enabled"))

	rootCmd.PersistentFlags().String("http.listen-addr", "", "Listen address for the HTTP server (e.g., :8080)")
	viper.BindPFlag("http.listen_addr", rootCmd.PersistentFlags().Lookup("http.listen-addr"))

	// MetricsConfig Flag
	rootCmd.PersistentFlags().Bool("metrics.enabled", false, "Expose the Prometheus /metrics endpoint on the HTTP server")
	viper.BindPFlag("metrics.enabled", rootCmd.PersistentFlags().Lookup("metrics.enabled"))
}

// Execute runs the root command.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Execution error: %v\n", err)
		os.Exit(1)
	}
}
