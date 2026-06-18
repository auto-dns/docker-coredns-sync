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

func TestNewServer_BindError(t *testing.T) {
	// Occupy an address, then try to bind the health server to the same one.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer ln.Close()

	if _, err := NewServer(ln.Addr().String(), NewStatus(time.Minute), zerolog.Nop()); err == nil {
		t.Error("expected NewServer to fail binding an in-use address")
	}
}

func TestServer_Close_FreesListenerWithoutStart(t *testing.T) {
	addr := freeAddr(t)
	srv, err := NewServer(addr, NewStatus(time.Minute), zerolog.Nop())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Close without ever calling Start must free the bound port.
	if err := srv.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatalf("expected port to be freed after Close, got: %v", err)
	}
	_ = ln.Close()

	// Double close is a no-op.
	if err := srv.Close(); err != nil {
		t.Errorf("expected double Close to be a no-op, got: %v", err)
	}
}

func TestServer_StartAndShutdown(t *testing.T) {
	status := NewStatus(time.Minute)
	status.SetDockerConnected(true)
	status.RecordReconcile(nil)

	addr := freeAddr(t)
	srv, err := NewServer(addr, status, zerolog.Nop())
	if err != nil {
		t.Fatalf("unexpected error creating server: %v", err)
	}

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
