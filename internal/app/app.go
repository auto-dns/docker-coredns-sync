package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/auto-dns/docker-coredns-sync/internal/config"
	"github.com/auto-dns/docker-coredns-sync/internal/core"
	"github.com/auto-dns/docker-coredns-sync/internal/event"
	"github.com/auto-dns/docker-coredns-sync/internal/httpserver"
	"github.com/auto-dns/docker-coredns-sync/internal/metrics"
	"github.com/auto-dns/docker-coredns-sync/internal/registry"
	"github.com/auto-dns/docker-coredns-sync/internal/state"
	dockerCli "github.com/docker/docker/client"
	"github.com/rs/zerolog"
	clientv3 "go.etcd.io/etcd/client/v3"
)

type App struct {
	dockerClient io.Closer
	etcdClient   io.Closer
	engine       *core.SyncEngine
	httpServer   *httpserver.Server
	status       *httpserver.Status
	logger       zerolog.Logger
}

type DockerClientFactory func() (*dockerCli.Client, error)
type EtcdClientFactory func(cfg *config.EtcdConfig, dialTimeout time.Duration) (*clientv3.Client, error)

type ClientFactories struct {
	DockerClientFactory DockerClientFactory
	EtcdClientFactory   EtcdClientFactory
}

func DefaultFactories() ClientFactories {
	return ClientFactories{
		DockerClientFactory: func() (*dockerCli.Client, error) {
			return dockerCli.NewClientWithOpts(dockerCli.FromEnv, dockerCli.WithAPIVersionNegotiation())
		},
		EtcdClientFactory: func(cfg *config.EtcdConfig, dialTimeout time.Duration) (*clientv3.Client, error) {
			tlsCfg, err := cfg.ClientTLS()
			if err != nil {
				return nil, err
			}
			return clientv3.New(clientv3.Config{
				Endpoints:   cfg.Endpoints,
				DialTimeout: dialTimeout,
				Username:    cfg.Username,
				Password:    cfg.Password,
				TLS:         tlsCfg,
			})
		},
	}
}

func NewWithFactories(cfg *config.Config, logger zerolog.Logger, factories ClientFactories) (*App, error) {
	dockerClient, err := factories.DockerClientFactory()
	if err != nil {
		return nil, err
	}

	// Warn when etcd credentials would be sent over an unencrypted connection.
	if cfg.Etcd.Username != "" && !cfg.Etcd.UsesTLS() {
		logger.Warn().Msg("etcd username/password configured without TLS or an https:// endpoint: credentials will be sent in plaintext")
	}

	// metrics is created when the /metrics endpoint is enabled; it is fed by the
	// engine (reconcile outcomes), the registry (etcd op/lock errors), and the
	// Docker generator (disconnects).
	var m *metrics.Metrics
	if cfg.Metrics.Enabled {
		m = metrics.New()
	}

	// Status backs the health endpoints; it is shared by the engine (reconcile
	// outcomes) and the Docker generator (connection state).
	var status *httpserver.Status
	if cfg.HTTP.Enabled {
		// Consider the daemon stale if a reconciliation has not succeeded within
		// a few poll intervals.
		readyThreshold := 3 * time.Duration(cfg.App.PollInterval) * time.Second
		status = httpserver.NewStatus(readyThreshold)
	}

	genOpts := []event.Option{
		event.WithEventBufferSize(cfg.Docker.EventBufferSize),
		event.WithReconnectBackoff(
			time.Duration(cfg.Docker.ReconnectInitialBackoff*float64(time.Second)),
			time.Duration(cfg.Docker.ReconnectMaxBackoff*float64(time.Second)),
		),
	}
	// Health tracks the full connection state; metrics counts only genuine
	// disconnects (the generator excludes clean shutdown and failed connects).
	if status != nil {
		genOpts = append(genOpts, event.WithConnectionObserver(status.SetDockerConnected))
	}
	if m != nil {
		genOpts = append(genOpts, event.WithDisconnectObserver(m.IncDockerDisconnect))
	}
	gen := event.NewDockerGenerator(dockerClient, logger, genOpts...)

	etcdClient, err := factories.EtcdClientFactory(&cfg.Etcd, 2*time.Second)
	if err != nil {
		_ = dockerClient.Close()
		return nil, fmt.Errorf("failed to connect to etcd: %w", err)
	}

	reg := registry.NewEtcdRegistry(etcdClient, &cfg.Etcd, cfg.App.Hostname, logger)
	if m != nil {
		reg.SetMetrics(m)
	}
	memState := state.NewMemoryState()
	engine := core.NewSyncEngine(logger, &cfg.App, gen, reg, memState)

	app := &App{
		dockerClient: dockerClient,
		etcdClient:   etcdClient,
		engine:       engine,
		logger:       logger,
	}

	// In dry-run the daemon intentionally writes nothing, so neither readiness
	// nor metrics should report it as a ready, successfully-syncing instance.
	if cfg.App.DryRun {
		logger.Warn().Msg("dry-run enabled: no records are applied; readiness reports not-ready")
	}
	if m != nil {
		engine.SetMetrics(m)
		m.SetDryRun(cfg.App.DryRun)
	}
	if status != nil {
		status.SetDryRun(cfg.App.DryRun)
		engine.SetReconcileReporter(status)
		app.status = status
	}

	// Start the shared HTTP server when either the health endpoints or the
	// metrics endpoint is enabled.
	if cfg.HTTPServerEnabled() {
		var metricsHandler http.Handler
		if m != nil {
			metricsHandler = m.Handler()
		}
		httpServer, err := httpserver.NewServer(cfg.HTTP.ListenAddr, status, metricsHandler, logger)
		if err != nil {
			_ = dockerClient.Close()
			_ = etcdClient.Close()
			return nil, err
		}
		app.httpServer = httpServer
	}

	return app, nil
}

func New(cfg *config.Config, logger zerolog.Logger) (*App, error) {
	return NewWithFactories(cfg, logger, DefaultFactories())
}

var _ io.Closer = (*App)(nil)

// Run starts the application by running the sync engine.
func (a *App) Run(ctx context.Context) error {
	a.logger.Info().Msg("Application starting")
	if a.httpServer != nil {
		a.httpServer.Start(ctx)
	}
	return a.engine.Run(ctx)
}

func (a *App) Close() error {
	var err error

	if a.dockerClient != nil {
		if e := a.dockerClient.Close(); e != nil {
			err = errors.Join(err, fmt.Errorf("close docker client: %w", e))
		}
	}
	if a.etcdClient != nil {
		if e := a.etcdClient.Close(); e != nil {
			err = errors.Join(err, fmt.Errorf("close etcd client: %w", e))
		}
	}
	if a.httpServer != nil {
		if e := a.httpServer.Close(); e != nil {
			err = errors.Join(err, fmt.Errorf("close HTTP server: %w", e))
		}
	}

	return err
}
