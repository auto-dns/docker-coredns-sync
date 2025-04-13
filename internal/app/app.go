package app

import (
	"context"
	"fmt"
	"time"

	"github.com/StevenC4/docker-coredns-sync/internal/config"
	"github.com/StevenC4/docker-coredns-sync/internal/core"
	"github.com/StevenC4/docker-coredns-sync/internal/registry"
	dockerCli "github.com/docker/docker/client"
	"github.com/rs/zerolog"
	clientv3 "go.etcd.io/etcd/client/v3"
)

type App struct {
	Config   *config.Config
	Logger   zerolog.Logger
	Registry registry.Registry
	Watcher  core.DockerWatcher
	Engine   *core.SyncEngine
}

func getNewWatcher(cfg *config.Config, logger zerolog.Logger) (core.DockerWatcher, error) {
	dockerClient, err := dockerCli.NewClientWithOpts(dockerCli.FromEnv, dockerCli.WithAPIVersionNegotiation())
	if err != nil {
		return nil, err
	}
	watcher, err := core.NewDockerWatcherImpl(dockerClient, logger)
	if err != nil {
		return nil, err
	}
	return watcher, nil
}

func getNewRegistry(cfg *config.Config, logger zerolog.Logger) (registry.Registry, error) {
	etcdClient, err := clientv3.New(clientv3.Config{
		Endpoints:   []string{fmt.Sprintf("%s:%d", cfg.Etcd.Host, cfg.Etcd.Port)},
		DialTimeout: 5 * time.Second,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to etcd: %w", err)
	}
	reg, err := registry.NewEtcdRegistry(etcdClient, &cfg.Etcd, cfg.App.Hostname, logger)
	if err != nil {
		return nil, err
	}
	return reg, nil
}

// New creates a new App by wiring up all dependencies.
func New(cfg *config.Config, logInstance zerolog.Logger) (*App, error) {
	watcher, err := getNewWatcher(cfg, logInstance)
	if err != nil {
		return nil, fmt.Errorf("failed to create Docker watcher: %w", err)
	}

	reg, err := getNewRegistry(cfg, logInstance)
	if err != nil {
		return nil, fmt.Errorf("failed to create etcd registry: %w", err)
	}

	engine := core.NewSyncEngine(logInstance, &cfg.App, watcher, reg)

	return &App{
		Config:   cfg,
		Logger:   logInstance,
		Registry: reg,
		Watcher:  watcher,
		Engine:   engine,
	}, nil
}

// Run starts the application by running the sync engine.
func (a *App) Run(ctx context.Context) error {
	a.Logger.Info().Msg("Application starting")
	return a.Engine.Run(ctx)
}
