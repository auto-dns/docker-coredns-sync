package main

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/spf13/viper"

	"github.com/auto-dns/docker-coredns-sync/internal/config"
)

type mockAppRunner struct {
	runFunc   func(ctx context.Context) error
	closeFunc func() error
	runCalled bool
}

func (m *mockAppRunner) Run(ctx context.Context) error {
	m.runCalled = true
	if m.runFunc != nil {
		return m.runFunc(ctx)
	}
	<-ctx.Done()
	return nil
}

func (m *mockAppRunner) Close() error {
	if m.closeFunc != nil {
		return m.closeFunc()
	}
	return nil
}

func testConfig() *config.Config {
	return &config.Config{
		App: config.AppConfig{
			DockerLabelPrefix: "coredns",
			HostIPv4:          "192.168.1.1",
			Hostname:          "test-host",
			PollInterval:      5,
		},
		Etcd: config.EtcdConfig{
			Endpoints:         []string{"http://localhost:2379"},
			PathPrefix:        "/skydns",
			LockTTL:           5.0,
			LockTimeout:       2.0,
			LockRetryInterval: 0.1,
		},
		Logging: config.LoggingConfig{
			Level: "INFO",
		},
	}
}

func TestRunWithDeps_Success(t *testing.T) {
	cfg := testConfig()

	mockApp := &mockAppRunner{
		runFunc: func(ctx context.Context) error {
			return nil
		},
	}

	factory := func(cfg *config.Config, log zerolog.Logger) (AppRunner, error) {
		return mockApp, nil
	}

	sigCh := make(chan os.Signal, 1)
	close(sigCh)

	err := runWithDeps(cfg, factory, sigCh)

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if !mockApp.runCalled {
		t.Error("expected Run to be called")
	}
}

func TestRunWithDeps_AppCreateError(t *testing.T) {
	cfg := testConfig()

	factory := func(cfg *config.Config, log zerolog.Logger) (AppRunner, error) {
		return nil, errors.New("failed to create app")
	}

	sigCh := make(chan os.Signal, 1)

	err := runWithDeps(cfg, factory, sigCh)

	if err == nil {
		t.Fatal("expected error for app creation failure")
	}
	if !strings.Contains(err.Error(), "failed to create app") {
		t.Errorf("expected app creation error message, got %v", err)
	}
}

func TestRunWithDeps_AppRunError(t *testing.T) {
	cfg := testConfig()

	mockApp := &mockAppRunner{
		runFunc: func(ctx context.Context) error {
			return errors.New("app run failed")
		},
	}

	factory := func(cfg *config.Config, log zerolog.Logger) (AppRunner, error) {
		return mockApp, nil
	}

	sigCh := make(chan os.Signal, 1)
	close(sigCh)

	err := runWithDeps(cfg, factory, sigCh)

	if err == nil {
		t.Fatal("expected error for app run failure")
	}
	if !strings.Contains(err.Error(), "app run error") {
		t.Errorf("expected app run error message, got %v", err)
	}
}

func TestRunWithDeps_SignalHandling(t *testing.T) {
	cfg := testConfig()

	runStarted := make(chan struct{})
	mockApp := &mockAppRunner{
		runFunc: func(ctx context.Context) error {
			close(runStarted)
			<-ctx.Done()
			return nil
		},
	}

	factory := func(cfg *config.Config, log zerolog.Logger) (AppRunner, error) {
		return mockApp, nil
	}

	sigCh := make(chan os.Signal, 1)

	done := make(chan error, 1)
	go func() {
		done <- runWithDeps(cfg, factory, sigCh)
	}()

	<-runStarted
	sigCh <- os.Interrupt

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("expected no error after signal, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for runWithDeps to complete")
	}
}

func TestRunWithDeps_CloseError(t *testing.T) {
	cfg := testConfig()

	mockApp := &mockAppRunner{
		runFunc: func(ctx context.Context) error {
			return nil
		},
		closeFunc: func() error {
			return errors.New("close failed")
		},
	}

	factory := func(cfg *config.Config, log zerolog.Logger) (AppRunner, error) {
		return mockApp, nil
	}

	sigCh := make(chan os.Signal, 1)
	close(sigCh)

	err := runWithDeps(cfg, factory, sigCh)

	// The close error is logged but not returned
	if err != nil {
		t.Errorf("expected no error (close errors are logged), got %v", err)
	}
}

func TestInit_FlagsRegistered(t *testing.T) {
	flags := rootCmd.PersistentFlags()

	expectedFlags := []string{
		"config",
		"app.docker-label-prefix",
		"app.host-ipv4",
		"app.host-ipv6",
		"app.hostname",
		"app.poll-interval",
		"etcd-endpoints",
		"etcd.path-prefix",
		"etcd.lock-ttl",
		"etcd.lock-timeout",
		"etcd.lock-retry-interval",
		"log.level",
	}

	for _, name := range expectedFlags {
		f := flags.Lookup(name)
		if f == nil {
			t.Errorf("expected flag %q to be registered", name)
		}
	}
}

func TestRootCmd_Properties(t *testing.T) {
	if rootCmd.Use != "docker-coredns-sync" {
		t.Errorf("expected Use to be 'docker-coredns-sync', got %q", rootCmd.Use)
	}
	if rootCmd.Short == "" {
		t.Error("expected Short description to be set")
	}
	if rootCmd.Long == "" {
		t.Error("expected Long description to be set")
	}
}

func resetViper() {
	viper.Reset()
}

func TestPersistentPreRunE_LoadsConfig(t *testing.T) {
	resetViper()
	defer resetViper()

	t.Setenv("DOCKER_COREDNS_SYNC_APP_HOSTNAME", "pre-run-host")

	ctx := context.Background()
	cmd := *rootCmd
	cmd.SetContext(ctx)

	err := cmd.PersistentPreRunE(&cmd, nil)

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	cfg := cmd.Context().Value(configKey).(*config.Config)
	if cfg == nil {
		t.Fatal("expected config to be set in context")
	}
	if cfg.App.Hostname != "pre-run-host" {
		t.Errorf("expected hostname 'pre-run-host', got %q", cfg.App.Hostname)
	}
}
