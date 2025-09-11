package app

import (
	"context"
	"fmt"
	"time"

	"github.com/auto-dns/docker-coredns-sync/internal/config"
	"github.com/auto-dns/docker-coredns-sync/internal/core"
	"github.com/auto-dns/docker-coredns-sync/internal/event"
	"github.com/auto-dns/docker-coredns-sync/internal/registry"
	"github.com/auto-dns/docker-coredns-sync/internal/state"
	dockerCli "github.com/docker/docker/client"
	"github.com/rs/zerolog"
	clientv3 "go.etcd.io/etcd/client/v3"
)

type App struct {
	dockerClient *dockerCli.Client
	etcdClient   *clientv3.Client
	engine       *core.SyncEngine
	logger       zerolog.Logger
}

// New creates a new App by wiring up all dependencies.
func New(cfg *config.Config, logger zerolog.Logger) (*App, error) {
	// Docker CLI
	dockerClient, err := dockerCli.NewClientWithOpts(dockerCli.FromEnv, dockerCli.WithAPIVersionNegotiation())
	if err != nil {
		return nil, err
	}
	gen := event.NewDockerGenerator(dockerClient, logger)

	// etcd CLI
	etcdClient, err := clientv3.New(clientv3.Config{
		Endpoints:   cfg.Etcd.Endpoints,
		DialTimeout: 2 * time.Second,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to etcd: %w", err)
	}

	// Engine
	reg := registry.NewEtcdRegistry(etcdClient, &cfg.Etcd, cfg.App.Hostname, logger)
	memState := state.NewMemoryState()
	engine := core.NewSyncEngine(logger, cfg.App, gen, reg, memState)

	return &App{
		dockerClient: dockerClient,
		etcdClient:   etcdClient,
		engine:       engine,
		logger:       logger,
	}, nil
}

// Run starts the application by running the sync engine.
func (a *App) Run(ctx context.Context) error {
	defer func() {}()
	a.logger.Info().Msg("Application starting")
	return a.engine.Run(ctx)
}

func (a *App) Close() error {
	var firstErr error
	if a.dockerClient != nil {
		if err := a.dockerClient.Close(); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("close docker client: %w", err)
		}
	}
	if a.etcdClient != nil {
		if err := a.etcdClient.Close(); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("close etcd client: %w", err)
		}
	}
	return firstErr
}
