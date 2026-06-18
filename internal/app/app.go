package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/auto-dns/docker-coredns-sync/internal/config"
	"github.com/auto-dns/docker-coredns-sync/internal/core"
	"github.com/auto-dns/docker-coredns-sync/internal/event"
	"github.com/auto-dns/docker-coredns-sync/internal/health"
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
	healthServer *health.Server
	logger       zerolog.Logger
}

type DockerClientFactory func() (*dockerCli.Client, error)
type EtcdClientFactory func(endpoints []string, dialTimeout time.Duration) (*clientv3.Client, error)

type ClientFactories struct {
	DockerClientFactory DockerClientFactory
	EtcdClientFactory   EtcdClientFactory
}

func DefaultFactories() ClientFactories {
	return ClientFactories{
		DockerClientFactory: func() (*dockerCli.Client, error) {
			return dockerCli.NewClientWithOpts(dockerCli.FromEnv, dockerCli.WithAPIVersionNegotiation())
		},
		EtcdClientFactory: func(endpoints []string, dialTimeout time.Duration) (*clientv3.Client, error) {
			return clientv3.New(clientv3.Config{
				Endpoints:   endpoints,
				DialTimeout: dialTimeout,
			})
		},
	}
}

func NewWithFactories(cfg *config.Config, logger zerolog.Logger, factories ClientFactories) (*App, error) {
	dockerClient, err := factories.DockerClientFactory()
	if err != nil {
		return nil, err
	}
	gen := event.NewDockerGenerator(dockerClient, logger)
	gen.SetEventBufferSize(cfg.Docker.EventBufferSize)
	gen.SetReconnectBackoff(
		time.Duration(cfg.Docker.ReconnectInitialBackoff*float64(time.Second)),
		time.Duration(cfg.Docker.ReconnectMaxBackoff*float64(time.Second)),
	)

	etcdClient, err := factories.EtcdClientFactory(cfg.Etcd.Endpoints, 2*time.Second)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to etcd: %w", err)
	}

	reg := registry.NewEtcdRegistry(etcdClient, &cfg.Etcd, cfg.App.Hostname, logger)
	memState := state.NewMemoryState()
	engine := core.NewSyncEngine(logger, &cfg.App, gen, reg, memState)

	app := &App{
		dockerClient: dockerClient,
		etcdClient:   etcdClient,
		engine:       engine,
		logger:       logger,
	}

	if cfg.HTTP.Enabled {
		// Consider the daemon stale if a reconciliation has not succeeded within
		// a few poll intervals.
		readyThreshold := 3 * time.Duration(cfg.App.PollInterval) * time.Second
		status := health.NewStatus(readyThreshold)
		engine.SetReconcileReporter(status)
		gen.SetConnectionObserver(status.SetDockerConnected)
		app.healthServer = health.NewServer(cfg.HTTP.ListenAddr, status, logger)
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
	if a.healthServer != nil {
		a.healthServer.Start(ctx)
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

	return err
}
