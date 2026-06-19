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
	gotChoices    []HostChoice
	selectHost    string
	selectErr     error
	confirm       bool
	confirmErr    error
	selectCalled  bool
	confirmCalled bool
}

func (f *fakePrompter) SelectHost(choices []HostChoice) (string, error) {
	f.selectCalled = true
	f.gotChoices = choices
	return f.selectHost, f.selectErr
}

func (f *fakePrompter) Confirm(label string) (bool, error) {
	f.confirmCalled = true
	return f.confirm, f.confirmErr
}

func factoryFor(d Decommissioner) DecommissionerFactory {
	return func(cfg *config.Config, log zerolog.Logger) (Decommissioner, error) {
		return d, nil
	}
}

func choiceNames(choices []HostChoice) map[string]HostChoice {
	m := make(map[string]HostChoice, len(choices))
	for _, c := range choices {
		m[c.Hostname] = c
	}
	return m
}

// --- non-interactive ---

func TestRunDecommission_NonInteractive_WithYesSkipsPrompts(t *testing.T) {
	cfg := testConfig()
	mock := &mockDecommissioner{removed: 3}
	prompter := &fakePrompter{}

	var buf bytes.Buffer
	err := runDecommissionWithDeps(context.Background(), cfg, "dead-host", factoryFor(mock), prompter, true, &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !mock.called || mock.hostname != "dead-host" {
		t.Errorf("expected DecommissionHost('dead-host'), got called=%v host=%q", mock.called, mock.hostname)
	}
	if prompter.selectCalled || prompter.confirmCalled {
		t.Error("expected no prompts with --yes and an explicit hostname")
	}
	if !mock.closeCalled {
		t.Error("expected Close to be called")
	}
	if out := buf.String(); !strings.Contains(out, "dead-host") || !strings.Contains(out, "3") {
		t.Errorf("expected summary mentioning host and count, got %q", out)
	}
}

func TestRunDecommission_NonInteractive_ConfirmYes(t *testing.T) {
	cfg := testConfig()
	mock := &mockDecommissioner{removed: 1}
	prompter := &fakePrompter{confirm: true}

	err := runDecommissionWithDeps(context.Background(), cfg, "dead-host", factoryFor(mock), prompter, false, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !prompter.confirmCalled {
		t.Error("expected a confirmation prompt without --yes")
	}
	if !mock.called {
		t.Error("expected DecommissionHost to be called after confirmation")
	}
}

func TestRunDecommission_NonInteractive_ConfirmNoAborts(t *testing.T) {
	cfg := testConfig()
	mock := &mockDecommissioner{}
	prompter := &fakePrompter{confirm: false}

	var buf bytes.Buffer
	err := runDecommissionWithDeps(context.Background(), cfg, "dead-host", factoryFor(mock), prompter, false, &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mock.called {
		t.Error("expected no deletion when the user declines confirmation")
	}
	if !strings.Contains(buf.String(), "Aborted") {
		t.Errorf("expected 'Aborted' message, got %q", buf.String())
	}
}

func TestRunDecommission_RefusesForeignActiveHeartbeat(t *testing.T) {
	cfg := testConfig() // local host == "test-host"
	mock := &mockDecommissioner{
		hosts: []registry.HostSummary{{Hostname: "live-host", HasMarker: true, ActiveHeartbeat: true, RecordCount: 2}},
	}
	prompter := &fakePrompter{}

	err := runDecommissionWithDeps(context.Background(), cfg, "live-host", factoryFor(mock), prompter, true, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "active heartbeat") {
		t.Fatalf("expected refusal for a foreign host with an active heartbeat, got %v", err)
	}
	if mock.called {
		t.Error("expected no deletion for a live foreign host")
	}
}

func TestRunDecommission_AllowsLocalActiveHeartbeat(t *testing.T) {
	cfg := testConfig() // local host == "test-host"
	mock := &mockDecommissioner{
		removed: 1,
		hosts:   []registry.HostSummary{{Hostname: "test-host", HasMarker: true, ActiveHeartbeat: true, RecordCount: 1}},
	}

	err := runDecommissionWithDeps(context.Background(), cfg, "test-host", factoryFor(mock), &fakePrompter{}, true, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !mock.called || mock.hostname != "test-host" {
		t.Error("expected the local host to be decommissionable even with an active heartbeat")
	}
}

func TestRunDecommission_AllowsForeignOptOut(t *testing.T) {
	cfg := testConfig()
	mock := &mockDecommissioner{
		removed: 4,
		hosts:   []registry.HostSummary{{Hostname: "opt-host", HasMarker: true, ActiveHeartbeat: false, RecordCount: 4}},
	}

	err := runDecommissionWithDeps(context.Background(), cfg, "opt-host", factoryFor(mock), &fakePrompter{}, true, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !mock.called {
		t.Error("expected a foreign opt-out host to be decommissionable")
	}
}

func TestRunDecommission_FactoryError(t *testing.T) {
	cfg := testConfig()
	factory := func(cfg *config.Config, log zerolog.Logger) (Decommissioner, error) {
		return nil, errors.New("connect failed")
	}
	err := runDecommissionWithDeps(context.Background(), cfg, "dead-host", factory, &fakePrompter{}, true, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "connect failed") {
		t.Fatalf("expected factory error to propagate, got %v", err)
	}
}

func TestRunDecommission_ListError(t *testing.T) {
	cfg := testConfig()
	mock := &mockDecommissioner{listErr: errors.New("etcd down")}
	err := runDecommissionWithDeps(context.Background(), cfg, "dead-host", factoryFor(mock), &fakePrompter{}, true, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "list hosts") {
		t.Fatalf("expected list hosts error, got %v", err)
	}
}

func TestRunDecommission_DecommissionError(t *testing.T) {
	cfg := testConfig()
	mock := &mockDecommissioner{err: errors.New("etcd unavailable")}
	err := runDecommissionWithDeps(context.Background(), cfg, "dead-host", factoryFor(mock), &fakePrompter{}, true, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "decommission") {
		t.Fatalf("expected decommission error, got %v", err)
	}
}

// --- interactive ---

func TestRunDecommission_Interactive_SelectsAndConfirms(t *testing.T) {
	cfg := testConfig() // local host == "test-host"
	mock := &mockDecommissioner{
		removed: 2,
		hosts: []registry.HostSummary{
			{Hostname: "test-host", RecordCount: 1, HasMarker: true, ActiveHeartbeat: true},
			{Hostname: "dead-host", RecordCount: 2, HasMarker: false},
		},
	}
	prompter := &fakePrompter{selectHost: "dead-host", confirm: true}

	err := runDecommissionWithDeps(context.Background(), cfg, "", factoryFor(mock), prompter, false, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !prompter.selectCalled || !prompter.confirmCalled {
		t.Error("expected both selection and confirmation prompts")
	}
	if !mock.called || mock.hostname != "dead-host" {
		t.Errorf("expected DecommissionHost('dead-host'), got called=%v host=%q", mock.called, mock.hostname)
	}

	choices := choiceNames(prompter.gotChoices)
	if c, ok := choices["test-host"]; !ok || !c.IsThisHost {
		t.Error("expected the local host to be offered and flagged IsThisHost")
	}
	if _, ok := choices["dead-host"]; !ok {
		t.Error("expected the dead host to be offered")
	}
}

func TestRunDecommission_Interactive_FiltersForeignActiveHosts(t *testing.T) {
	cfg := testConfig()
	mock := &mockDecommissioner{
		removed: 1,
		hosts: []registry.HostSummary{
			{Hostname: "test-host", HasMarker: true, ActiveHeartbeat: true}, // local: still offered
			{Hostname: "live-foreign", HasMarker: true, ActiveHeartbeat: true},
			{Hostname: "dead-foreign", RecordCount: 3},
		},
	}
	prompter := &fakePrompter{selectHost: "dead-foreign", confirm: true}

	err := runDecommissionWithDeps(context.Background(), cfg, "", factoryFor(mock), prompter, false, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	choices := choiceNames(prompter.gotChoices)
	if _, ok := choices["live-foreign"]; ok {
		t.Error("a foreign host with an active heartbeat must not be offered")
	}
	if _, ok := choices["test-host"]; !ok {
		t.Error("the local host must be offered even with an active heartbeat")
	}
	if _, ok := choices["dead-foreign"]; !ok {
		t.Error("an eligible foreign host must be offered")
	}
}

func TestRunDecommission_Interactive_NoEligibleHosts(t *testing.T) {
	cfg := testConfig()
	mock := &mockDecommissioner{
		hosts: []registry.HostSummary{{Hostname: "live-foreign", HasMarker: true, ActiveHeartbeat: true}},
	}
	prompter := &fakePrompter{}

	var buf bytes.Buffer
	err := runDecommissionWithDeps(context.Background(), cfg, "", factoryFor(mock), prompter, false, &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if prompter.selectCalled || mock.called {
		t.Error("expected no selection or deletion when no hosts are eligible")
	}
	if !strings.Contains(buf.String(), "No hosts are eligible") {
		t.Errorf("expected an ineligible message, got %q", buf.String())
	}
}

func TestRunDecommission_Interactive_AbortOnNoConfirm(t *testing.T) {
	cfg := testConfig()
	mock := &mockDecommissioner{
		hosts: []registry.HostSummary{{Hostname: "dead-host", RecordCount: 1}},
	}
	prompter := &fakePrompter{selectHost: "dead-host", confirm: false}

	var buf bytes.Buffer
	err := runDecommissionWithDeps(context.Background(), cfg, "", factoryFor(mock), prompter, false, &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mock.called {
		t.Error("expected no deletion when the user declines")
	}
	if !strings.Contains(buf.String(), "Aborted") {
		t.Errorf("expected 'Aborted' message, got %q", buf.String())
	}
}

func TestRunDecommission_Interactive_SelectError(t *testing.T) {
	cfg := testConfig()
	mock := &mockDecommissioner{hosts: []registry.HostSummary{{Hostname: "dead-host"}}}
	prompter := &fakePrompter{selectErr: errors.New("cancelled")}
	err := runDecommissionWithDeps(context.Background(), cfg, "", factoryFor(mock), prompter, false, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "cancelled") {
		t.Fatalf("expected select error to propagate, got %v", err)
	}
	if mock.called {
		t.Error("expected no deletion when selection fails")
	}
}

func TestFormatHostChoice(t *testing.T) {
	local := formatHostChoice(HostChoice{Hostname: "node-1", RecordCount: 3, HasMarker: true, ActiveHeartbeat: true, IsThisHost: true})
	if !strings.Contains(local, "This host (node-1)") {
		t.Errorf("expected local host label, got %q", local)
	}
	if !strings.Contains(local, "3 record(s)") || !strings.Contains(local, "active heartbeat") {
		t.Errorf("expected record count and active-heartbeat status, got %q", local)
	}

	optOut := formatHostChoice(HostChoice{Hostname: "node-2", RecordCount: 1, HasMarker: true, ActiveHeartbeat: false})
	if !strings.Contains(optOut, "opt-out marker") {
		t.Errorf("expected opt-out status, got %q", optOut)
	}

	dead := formatHostChoice(HostChoice{Hostname: "node-3", RecordCount: 0, HasMarker: false})
	if strings.Contains(dead, "This host") || !strings.Contains(dead, "no heartbeat") {
		t.Errorf("expected a plain peer with 'no heartbeat', got %q", dead)
	}
}

func TestDecommissionCmd_Registered(t *testing.T) {
	var cmd interface{ Name() string }
	found := false
	for _, c := range rootCmd.Commands() {
		if c.Name() == "decommission" {
			found = true
			cmd = c
			if c.Flags().Lookup("yes") == nil {
				t.Error("expected --yes flag on decommission command")
			}
		}
	}
	if !found {
		t.Error("expected 'decommission' subcommand to be registered")
	}
	_ = cmd
}
