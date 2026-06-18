package main

import (
	"context"
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

	if err := application.Run(ctx); err != nil {
		return fmt.Errorf("app run error: %w", err)
	}
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

	// EtcdConfig Flags
	rootCmd.PersistentFlags().StringArray("etcd-endpoints", []string{"http://localhost:2379"}, "etcd endpoints to connect to (can specify multiple times)")
	viper.BindPFlag("etcd.endpoints", rootCmd.PersistentFlags().Lookup("etcd-endpoints"))

	rootCmd.PersistentFlags().String("etcd.path-prefix", "", "etcd key path prefix (e.g., /skydns)")
	viper.BindPFlag("etcd.path_prefix", rootCmd.PersistentFlags().Lookup("etcd.path-prefix"))

	rootCmd.PersistentFlags().Float64("etcd.lock-ttl", 0, "TTL (in seconds) for etcd locks")
	viper.BindPFlag("etcd.lock_ttl", rootCmd.PersistentFlags().Lookup("etcd.lock-ttl"))

	rootCmd.PersistentFlags().Float64("etcd.lock-timeout", 0, "Timeout (in seconds) for acquiring etcd locks")
	viper.BindPFlag("etcd.lock_timeout", rootCmd.PersistentFlags().Lookup("etcd.lock-timeout"))

	rootCmd.PersistentFlags().Float64("etcd.lock-retry-interval", 0, "Interval (in seconds) to retry etcd lock acquisition")
	viper.BindPFlag("etcd.lock_retry_interval", rootCmd.PersistentFlags().Lookup("etcd.lock-retry-interval"))

	// LoggingConfig Flag
	rootCmd.PersistentFlags().String("log.level", "", "Log level (e.g., TRACE, DEBUG, INFO, WARN, ERROR, FATAL)")
	viper.BindPFlag("log.level", rootCmd.PersistentFlags().Lookup("log.level"))
}

// Execute runs the root command.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Execution error: %v\n", err)
		os.Exit(1)
	}
}
