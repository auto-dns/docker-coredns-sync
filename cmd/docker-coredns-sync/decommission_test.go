package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/rs/zerolog"

	"github.com/auto-dns/docker-coredns-sync/internal/config"
	"github.com/auto-dns/docker-coredns-sync/internal/registry"
)

type mockDecommissioner struct {
	hosts    []registry.HostSummary
	listErr  error
	removed  int
	err      error
	closeErr error

	listCalled  bool
	called      bool
	hostname    string
	closeCalled bool
}

func (m *mockDecommissioner) ListHosts(ctx context.Context) ([]registry.HostSummary, error) {
	m.listCalled = true
	return m.hosts, m.listErr
}

func (m *mockDecommissioner) DecommissionHost(ctx context.Context, hostname string) (int, error) {
	m.called = true
	m.hostname = hostname
	return m.removed, m.err
}

func (m *mockDecommissioner) Close() error {
	m.closeCalled = true
	return m.closeErr
}

type fakePrompter struct {
	gotChoices   []HostChoice
	selectHost   string
	selectErr    error
	confirm      bool
	confirmErr   error
	selectCalled bool
}

func (f *fakePrompter) SelectHost(choices []HostChoice) (string, error) {
	f.selectCalled = true
	f.gotChoices = choices
	return f.selectHost, f.selectErr
}

func (f *fakePrompter) Confirm(label string) (bool, error) {
	return f.confirm, f.confirmErr
}

func factoryFor(d Decommissioner) DecommissionerFactory {
	return func(cfg *config.Config, log zerolog.Logger) (Decommissioner, error) {
		return d, nil
	}
}

func TestRunDecommission_NonInteractive_Success(t *testing.T) {
	cfg := testConfig()
	mock := &mockDecommissioner{removed: 3}
	prompter := &fakePrompter{}

	var buf bytes.Buffer
	err := runDecommissionWithDeps(context.Background(), cfg, "dead-host", factoryFor(mock), prompter, &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !mock.called || mock.hostname != "dead-host" {
		t.Errorf("expected DecommissionHost('dead-host'), got called=%v host=%q", mock.called, mock.hostname)
	}
	if prompter.selectCalled {
		t.Error("expected no interactive prompt when a hostname is given")
	}
	if !mock.closeCalled {
		t.Error("expected Close to be called")
	}
	if out := buf.String(); !strings.Contains(out, "dead-host") || !strings.Contains(out, "3") {
		t.Errorf("expected summary mentioning host and count, got %q", out)
	}
}

func TestRunDecommission_FactoryError(t *testing.T) {
	cfg := testConfig()
	factory := func(cfg *config.Config, log zerolog.Logger) (Decommissioner, error) {
		return nil, errors.New("connect failed")
	}
	err := runDecommissionWithDeps(context.Background(), cfg, "dead-host", factory, &fakePrompter{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "connect failed") {
		t.Fatalf("expected factory error to propagate, got %v", err)
	}
}

func TestRunDecommission_DecommissionError(t *testing.T) {
	cfg := testConfig()
	mock := &mockDecommissioner{err: errors.New("etcd unavailable")}
	err := runDecommissionWithDeps(context.Background(), cfg, "dead-host", factoryFor(mock), &fakePrompter{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "decommission") {
		t.Fatalf("expected decommission error, got %v", err)
	}
}

func TestRunDecommission_Interactive_SelectsAndConfirms(t *testing.T) {
	cfg := testConfig() // App.Hostname == "test-host"
	mock := &mockDecommissioner{
		removed: 2,
		hosts: []registry.HostSummary{
			{Hostname: "test-host", RecordCount: 1, HasMarker: true},
			{Hostname: "dead-host", RecordCount: 2, HasMarker: false},
		},
	}
	prompter := &fakePrompter{selectHost: "dead-host", confirm: true}

	var buf bytes.Buffer
	err := runDecommissionWithDeps(context.Background(), cfg, "", factoryFor(mock), prompter, &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !mock.listCalled {
		t.Error("expected ListHosts to be called in interactive mode")
	}
	if !prompter.selectCalled {
		t.Error("expected SelectHost to be called")
	}
	if !mock.called || mock.hostname != "dead-host" {
		t.Errorf("expected DecommissionHost('dead-host'), got called=%v host=%q", mock.called, mock.hostname)
	}

	// The local host must be flagged so the UI can label it "This host (...)".
	var thisHostFlagged bool
	for _, c := range prompter.gotChoices {
		if c.Hostname == "test-host" {
			thisHostFlagged = c.IsThisHost
		}
		if c.Hostname == "dead-host" && c.IsThisHost {
			t.Error("dead-host should not be flagged as this host")
		}
	}
	if !thisHostFlagged {
		t.Error("expected the local host (test-host) to be flagged IsThisHost")
	}
}

func TestRunDecommission_Interactive_AbortOnNoConfirm(t *testing.T) {
	cfg := testConfig()
	mock := &mockDecommissioner{
		hosts: []registry.HostSummary{{Hostname: "dead-host", RecordCount: 1}},
	}
	prompter := &fakePrompter{selectHost: "dead-host", confirm: false}

	var buf bytes.Buffer
	err := runDecommissionWithDeps(context.Background(), cfg, "", factoryFor(mock), prompter, &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mock.called {
		t.Error("expected DecommissionHost NOT to be called when the user declines")
	}
	if !strings.Contains(buf.String(), "Aborted") {
		t.Errorf("expected an 'Aborted' message, got %q", buf.String())
	}
}

func TestRunDecommission_Interactive_NoHosts(t *testing.T) {
	cfg := testConfig()
	mock := &mockDecommissioner{hosts: nil}
	prompter := &fakePrompter{}

	var buf bytes.Buffer
	err := runDecommissionWithDeps(context.Background(), cfg, "", factoryFor(mock), prompter, &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if prompter.selectCalled || mock.called {
		t.Error("expected no selection or deletion when there are no hosts")
	}
	if !strings.Contains(buf.String(), "No hosts found") {
		t.Errorf("expected a 'No hosts found' message, got %q", buf.String())
	}
}

func TestRunDecommission_Interactive_ListError(t *testing.T) {
	cfg := testConfig()
	mock := &mockDecommissioner{listErr: errors.New("etcd down")}
	err := runDecommissionWithDeps(context.Background(), cfg, "", factoryFor(mock), &fakePrompter{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "list hosts") {
		t.Fatalf("expected list hosts error, got %v", err)
	}
}

func TestRunDecommission_Interactive_SelectError(t *testing.T) {
	cfg := testConfig()
	mock := &mockDecommissioner{hosts: []registry.HostSummary{{Hostname: "dead-host"}}}
	prompter := &fakePrompter{selectErr: errors.New("cancelled")}
	err := runDecommissionWithDeps(context.Background(), cfg, "", factoryFor(mock), prompter, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "cancelled") {
		t.Fatalf("expected select error to propagate, got %v", err)
	}
	if mock.called {
		t.Error("expected no deletion when selection fails")
	}
}

func TestFormatHostChoice(t *testing.T) {
	this := formatHostChoice(HostChoice{Hostname: "node-1", RecordCount: 3, HasMarker: true, IsThisHost: true})
	if !strings.Contains(this, "This host (node-1)") {
		t.Errorf("expected local host label, got %q", this)
	}
	if !strings.Contains(this, "3 record(s)") {
		t.Errorf("expected record count, got %q", this)
	}

	other := formatHostChoice(HostChoice{Hostname: "node-2", RecordCount: 0, HasMarker: false})
	if strings.Contains(other, "This host") {
		t.Errorf("did not expect local host label for a peer, got %q", other)
	}
	if !strings.Contains(other, "no heartbeat") {
		t.Errorf("expected 'no heartbeat' note for a markerless host, got %q", other)
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
