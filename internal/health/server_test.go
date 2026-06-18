package health

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

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
