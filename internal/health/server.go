package health

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/rs/zerolog"
)

// Handler returns the HTTP handler serving the health and readiness endpoints:
//   - GET /healthz — liveness; always 200 while the process is running.
//   - GET /readyz  — readiness; 200 when Status.Ready(), else 503 with reason.
func Handler(status *Status) http.Handler {
	mux := http.NewServeMux()
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
	return mux
}

// Server is the auxiliary HTTP server exposing the health endpoints.
type Server struct {
	srv      *http.Server
	listener net.Listener
	logger   zerolog.Logger
}

// NewServer binds a health server to addr. Binding happens synchronously so a
// bad or in-use address fails fast at startup rather than silently leaving the
// endpoints unavailable.
func NewServer(addr string, status *Status, logger zerolog.Logger) (*Server, error) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("bind health server on %q: %w", addr, err)
	}
	return &Server{
		srv: &http.Server{
			Handler:           Handler(status),
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
			s.logger.Error().Err(err).Msg("error shutting down health server")
		}
	}()
	go func() {
		s.logger.Info().Str("addr", s.listener.Addr().String()).Msg("Starting health/readiness server")
		if err := s.srv.Serve(s.listener); err != nil && err != http.ErrServerClosed {
			s.logger.Error().Err(err).Msg("health server error")
		}
	}()
}
