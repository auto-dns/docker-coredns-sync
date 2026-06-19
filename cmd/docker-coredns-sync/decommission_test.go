package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/rs/zerolog"

	"github.com/auto-dns/docker-coredns-sync/internal/config"
)

type mockDecommissioner struct {
	removed  int
	err      error
	closeErr error
	called   bool
	hostname string
}

func (m *mockDecommissioner) DecommissionHost(ctx context.Context, hostname string) (int, error) {
	m.called = true
	m.hostname = hostname
	return m.removed, m.err
}

func (m *mockDecommissioner) Close() error { return m.closeErr }

func TestRunDecommission_Success(t *testing.T) {
	cfg := testConfig()
	mock := &mockDecommissioner{removed: 3}
	factory := func(cfg *config.Config, log zerolog.Logger) (Decommissioner, error) {
		return mock, nil
	}

	var buf bytes.Buffer
	err := runDecommissionWithDeps(context.Background(), cfg, "dead-host", factory, &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !mock.called {
		t.Error("expected DecommissionHost to be called")
	}
	if mock.hostname != "dead-host" {
		t.Errorf("expected target hostname 'dead-host', got %q", mock.hostname)
	}
	out := buf.String()
	if !strings.Contains(out, "dead-host") || !strings.Contains(out, "3") {
		t.Errorf("expected summary mentioning host and count, got %q", out)
	}
}

func TestRunDecommission_TrimsAndRejectsEmptyHostname(t *testing.T) {
	cfg := testConfig()
	factoryCalled := false
	factory := func(cfg *config.Config, log zerolog.Logger) (Decommissioner, error) {
		factoryCalled = true
		return &mockDecommissioner{}, nil
	}

	err := runDecommissionWithDeps(context.Background(), cfg, "   ", factory, &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected error for empty hostname")
	}
	if factoryCalled {
		t.Error("expected factory not to be called for an empty hostname")
	}
}

func TestRunDecommission_FactoryError(t *testing.T) {
	cfg := testConfig()
	factory := func(cfg *config.Config, log zerolog.Logger) (Decommissioner, error) {
		return nil, errors.New("connect failed")
	}

	err := runDecommissionWithDeps(context.Background(), cfg, "dead-host", factory, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "connect failed") {
		t.Fatalf("expected factory error to propagate, got %v", err)
	}
}

func TestRunDecommission_DecommissionError(t *testing.T) {
	cfg := testConfig()
	mock := &mockDecommissioner{err: errors.New("etcd unavailable")}
	factory := func(cfg *config.Config, log zerolog.Logger) (Decommissioner, error) {
		return mock, nil
	}

	err := runDecommissionWithDeps(context.Background(), cfg, "dead-host", factory, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "decommission") {
		t.Fatalf("expected decommission error, got %v", err)
	}
}

func TestDecommissionCmd_Registered(t *testing.T) {
	found := false
	for _, c := range rootCmd.Commands() {
		if c.Name() == "decommission" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected 'decommission' subcommand to be registered")
	}
}
