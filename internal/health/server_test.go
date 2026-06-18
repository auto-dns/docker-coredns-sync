package health

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

// freeAddr returns a loopback address that was free a moment ago.
func freeAddr(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to find free port: %v", err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()
	return addr
}

func TestServer_StartAndShutdown(t *testing.T) {
	status := NewStatus(time.Minute)
	status.SetDockerConnected(true)
	status.RecordReconcile(nil)

	addr := freeAddr(t)
	srv := NewServer(addr, status, zerolog.Nop())

	ctx, cancel := context.WithCancel(context.Background())
	srv.Start(ctx)

	// Poll until the server is accepting requests.
	url := "http://" + addr + "/healthz"
	var ok bool
	for i := 0; i < 50; i++ {
		resp, err := http.Get(url)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				ok = true
				break
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !ok {
		t.Fatal("server did not become ready")
	}

	// Cancelling the context should shut the server down.
	cancel()
	var stopped bool
	for i := 0; i < 50; i++ {
		if _, err := http.Get(url); err != nil {
			stopped = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !stopped {
		t.Error("expected server to stop serving after context cancellation")
	}
}

func TestHandler_Healthz_AlwaysOK(t *testing.T) {
	s := NewStatus(time.Minute) // not ready
	srv := httptest.NewServer(Handler(s))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/healthz")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 from /healthz, got %d", resp.StatusCode)
	}
}

func TestHandler_Readyz_503WhenNotReady(t *testing.T) {
	s := NewStatus(time.Minute)
	srv := httptest.NewServer(Handler(s))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/readyz")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("expected 503 from /readyz when not ready, got %d", resp.StatusCode)
	}
}

func TestHandler_Readyz_200WhenReady(t *testing.T) {
	s := NewStatus(time.Minute)
	s.SetDockerConnected(true)
	s.RecordReconcile(nil)

	srv := httptest.NewServer(Handler(s))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/readyz")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 from /readyz when ready, got %d", resp.StatusCode)
	}
}
