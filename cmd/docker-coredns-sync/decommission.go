package main

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"github.com/spf13/cobra"

	"github.com/auto-dns/docker-coredns-sync/internal/app"
	"github.com/auto-dns/docker-coredns-sync/internal/config"
	"github.com/auto-dns/docker-coredns-sync/internal/logger"
	"github.com/auto-dns/docker-coredns-sync/internal/registry"
)

// Decommissioner removes a host's records and heartbeat/opt-out marker from the
// registry. *registry.EtcdRegistry satisfies this interface.
type Decommissioner interface {
	DecommissionHost(ctx context.Context, hostname string) (int, error)
	Close() error
}

// DecommissionerFactory builds a Decommissioner from config. It is a seam for testing.
type DecommissionerFactory func(cfg *config.Config, log zerolog.Logger) (Decommissioner, error)

var defaultDecommissionerFactory DecommissionerFactory = func(cfg *config.Config, log zerolog.Logger) (Decommissioner, error) {
	cli, err := app.DefaultFactories().EtcdClientFactory(&cfg.Etcd, 5*time.Second)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to etcd: %w", err)
	}
	// heartbeatTTL is irrelevant here — this command never starts a heartbeat.
	return registry.NewEtcdRegistry(cli, &cfg.Etcd, cfg.App.Hostname, 0, log), nil
}

var decommissionCmd = &cobra.Command{
	Use:   "decommission <hostname>",
	Short: "Remove a host's DNS records and heartbeat marker from etcd",
	Long: `Decommission permanently removes a host from the shared etcd registry:
it deletes that host's heartbeat (or opt-out) marker and every DNS record it
owns.

Run it after the target host's docker-coredns-sync daemon has been stopped — a
still-running daemon would simply re-publish its marker and records. It can be
run from the host being removed or from any other machine that can reach the
same etcd cluster, and it is safe to run more than once.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg := cmd.Context().Value(configKey).(*config.Config)
		return runDecommissionWithDeps(cmd.Context(), cfg, args[0], defaultDecommissionerFactory, cmd.OutOrStdout())
	},
}

func init() {
	rootCmd.AddCommand(decommissionCmd)
}

func runDecommissionWithDeps(ctx context.Context, cfg *config.Config, hostname string, factory DecommissionerFactory, out io.Writer) error {
	log := logger.SetupLogger(&cfg.Logging)

	hostname = strings.TrimSpace(hostname)
	if hostname == "" {
		return fmt.Errorf("hostname must not be empty")
	}

	d, err := factory(cfg, log)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := d.Close(); cerr != nil {
			log.Error().Err(cerr).Msg("error closing etcd client")
		}
	}()

	removed, err := d.DecommissionHost(ctx, hostname)
	if err != nil {
		return fmt.Errorf("decommission %q: %w", hostname, err)
	}

	fmt.Fprintf(out, "Decommissioned %q: removed its heartbeat/opt-out marker and %d DNS record(s).\n", hostname, removed)
	return nil
}
