package httpserver

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/rs/zerolog"
)

// Handler returns the HTTP handler for the auxiliary server. The registered
// routes depend on which features are enabled:
//   - When status is non-nil:
//   - GET /healthz — liveness; always 200 while the process is running.
//   - GET /readyz  — readiness; 200 when Status.Ready(), else 503 with reason.
//   - When metricsHandler is non-nil:
//   - GET /metrics — Prometheus exposition.
//
// Either argument may be nil; at least one is expected to be set by the caller.
func Handler(status *Status, metricsHandler http.Handler) http.Handler {
	mux := http.NewServeMux()
	if status != nil {
		mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok"))
		})
		mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
			ready, reason := status.Ready()
			if !ready {
				w.WriteHeader(http.StatusServiceUnavailable)
				_, _ = w.Write([]byte(reason))
				return
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok"))
		})
	}
	if metricsHandler != nil {
		mux.Handle("/metrics", metricsHandler)
	}
	return mux
}

// Server is the auxiliary HTTP server exposing the health and metrics endpoints.
type Server struct {
	srv      *http.Server
	listener net.Listener
	logger   zerolog.Logger
}

// NewServer binds the auxiliary HTTP server to addr. Binding happens
// synchronously so a bad or in-use address fails fast at startup rather than
// silently leaving the endpoints unavailable. status and metricsHandler are
// passed through to Handler; either may be nil.
func NewServer(addr string, status *Status, metricsHandler http.Handler, logger zerolog.Logger) (*Server, error) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("bind HTTP server on %q: %w", addr, err)
	}
	return &Server{
		srv: &http.Server{
			Handler:           Handler(status, metricsHandler),
			ReadHeaderTimeout: 5 * time.Second,
		},
		listener: ln,
		logger:   logger,
	}, nil
}

// Start serves requests in the background and shuts down gracefully when ctx is
// cancelled.
func (s *Server) Start(ctx context.Context) {
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := s.srv.Shutdown(shutdownCtx); err != nil {
			s.logger.Error().Err(err).Msg("error shutting down HTTP server")
		}
	}()
	go func() {
		s.logger.Info().Str("addr", s.listener.Addr().String()).Msg("Starting auxiliary HTTP server")
		if err := s.srv.Serve(s.listener); err != nil &&
			!errors.Is(err, http.ErrServerClosed) && !errors.Is(err, net.ErrClosed) {
			s.logger.Error().Err(err).Msg("HTTP server error")
		}
	}()
}

// Close releases the server's listener. It is safe to call whether or not
// Start has run, and is a no-op if the listener is already closed (e.g. by a
// graceful ctx-driven shutdown).
func (s *Server) Close() error {
	if err := s.listener.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
		return err
	}
	return nil
}
