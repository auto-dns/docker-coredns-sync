package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/manifoldco/promptui"
	"github.com/rs/zerolog"
	"github.com/spf13/cobra"

	"github.com/auto-dns/docker-coredns-sync/internal/app"
	"github.com/auto-dns/docker-coredns-sync/internal/config"
	"github.com/auto-dns/docker-coredns-sync/internal/logger"
	"github.com/auto-dns/docker-coredns-sync/internal/registry"
)

// Decommissioner removes a host's records and heartbeat/opt-out marker from the
// registry and can enumerate known hosts. *registry.EtcdRegistry satisfies it.
type Decommissioner interface {
	ListHosts(ctx context.Context) ([]registry.HostSummary, error)
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

// HostChoice is a host presented for interactive selection.
type HostChoice struct {
	Hostname        string
	RecordCount     int
	HasMarker       bool
	ActiveHeartbeat bool
	IsThisHost      bool
}

// hostPrompter abstracts the interactive UI so the command flow stays testable.
type hostPrompter interface {
	SelectHost(choices []HostChoice) (string, error)
	Confirm(label string) (bool, error)
}

var decommissionCmd = &cobra.Command{
	Use:   "decommission [hostname]",
	Short: "Remove a host's DNS records and heartbeat marker from etcd",
	Long: `Decommission permanently removes a host from the shared etcd registry:
it deletes that host's heartbeat (or opt-out) marker and every DNS record it
owns.

With no argument it lists the known hosts and lets you pick one with the arrow
keys (the local host is shown as "This host (<hostname>)"). Pass a hostname
argument to skip the prompt — required when there is no interactive terminal,
e.g. inside a container or CI.

Run it after the target host's docker-coredns-sync daemon has been stopped — a
still-running daemon would simply re-publish its marker and records. It can be
run from the host being removed or from any other machine that can reach the
same etcd cluster, and it is safe to run more than once.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg := cmd.Context().Value(configKey).(*config.Config)
		hostname := ""
		if len(args) == 1 {
			hostname = args[0]
		}
		skipConfirm, _ := cmd.Flags().GetBool("yes")
		return runDecommissionWithDeps(cmd.Context(), cfg, hostname, defaultDecommissionerFactory, &promptuiPrompter{}, skipConfirm, cmd.OutOrStdout())
	},
}

func init() {
	decommissionCmd.Flags().BoolP("yes", "y", false, "Skip the confirmation prompt (required for non-interactive use)")
	rootCmd.AddCommand(decommissionCmd)
}

// eligibleToDecommission reports whether a host may be decommissioned. A foreign
// host with a live heartbeat is still running and must not be removed; the local
// host is always eligible (the operator is expected to have stopped its daemon).
func eligibleToDecommission(h registry.HostSummary, isLocal bool) bool {
	return isLocal || !h.ActiveHeartbeat
}

func runDecommissionWithDeps(ctx context.Context, cfg *config.Config, hostname string, factory DecommissionerFactory, prompter hostPrompter, skipConfirm bool, out io.Writer) error {
	log := logger.SetupLogger(&cfg.Logging)

	d, err := factory(cfg, log)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := d.Close(); cerr != nil {
			log.Error().Err(cerr).Msg("error closing etcd client")
		}
	}()

	hosts, err := d.ListHosts(ctx)
	if err != nil {
		return fmt.Errorf("list hosts: %w", err)
	}
	byName := make(map[string]registry.HostSummary, len(hosts))
	for _, h := range hosts {
		byName[h.Hostname] = h
	}

	hostname = strings.TrimSpace(hostname)

	if hostname == "" {
		// Interactive selection — offer only hosts that may be decommissioned.
		choices := make([]HostChoice, 0, len(hosts))
		for _, h := range hosts {
			isLocal := h.Hostname == cfg.App.Hostname
			if !eligibleToDecommission(h, isLocal) {
				continue
			}
			choices = append(choices, HostChoice{
				Hostname:        h.Hostname,
				RecordCount:     h.RecordCount,
				HasMarker:       h.HasMarker,
				ActiveHeartbeat: h.ActiveHeartbeat,
				IsThisHost:      isLocal,
			})
		}
		if len(choices) == 0 {
			fmt.Fprintln(out, "No hosts are eligible for decommissioning (a foreign host with an active heartbeat can't be removed while it's running).")
			return nil
		}

		selected, err := prompter.SelectHost(choices)
		if err != nil {
			return err
		}
		hostname = selected
	} else if h, ok := byName[hostname]; ok {
		if !eligibleToDecommission(h, hostname == cfg.App.Hostname) {
			return fmt.Errorf("refusing to decommission %q: it has an active heartbeat (its daemon appears to be running) — stop it first", hostname)
		}
	}

	if !skipConfirm {
		ok, err := prompter.Confirm(fmt.Sprintf("Decommission %q? This deletes its marker and all of its records", hostname))
		if err != nil {
			return err
		}
		if !ok {
			fmt.Fprintln(out, "Aborted.")
			return nil
		}
	}

	removed, err := d.DecommissionHost(ctx, hostname)
	if err != nil {
		return fmt.Errorf("decommission %q: %w", hostname, err)
	}

	fmt.Fprintf(out, "Decommissioned %q: removed its heartbeat/opt-out marker and %d DNS record(s).\n", hostname, removed)
	return nil
}

// formatHostChoice renders a host menu entry, labelling the local host clearly
// and noting its heartbeat status.
func formatHostChoice(h HostChoice) string {
	name := h.Hostname
	if h.IsThisHost {
		name = fmt.Sprintf("This host (%s)", h.Hostname)
	}
	var status string
	switch {
	case h.ActiveHeartbeat:
		status = "active heartbeat"
	case h.HasMarker:
		status = "opt-out marker"
	default:
		status = "no heartbeat"
	}
	return fmt.Sprintf("%s — %d record(s), %s", name, h.RecordCount, status)
}

// promptuiPrompter is the real terminal UI, backed by promptui.
type promptuiPrompter struct{}

func (p *promptuiPrompter) SelectHost(choices []HostChoice) (string, error) {
	items := make([]string, len(choices))
	for i, c := range choices {
		items[i] = formatHostChoice(c)
	}
	sel := promptui.Select{
		Label: "Select a host to decommission",
		Items: items,
		Size:  10,
	}
	idx, _, err := sel.Run()
	if err != nil {
		return "", err
	}
	return choices[idx].Hostname, nil
}

func (p *promptuiPrompter) Confirm(label string) (bool, error) {
	prompt := promptui.Prompt{Label: label, IsConfirm: true}
	if _, err := prompt.Run(); err != nil {
		if errors.Is(err, promptui.ErrAbort) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}
