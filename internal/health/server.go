package health

import (
	"context"
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
	srv    *http.Server
	logger zerolog.Logger
}

// NewServer builds a health server bound to addr.
func NewServer(addr string, status *Status, logger zerolog.Logger) *Server {
	return &Server{
		srv: &http.Server{
			Addr:              addr,
			Handler:           Handler(status),
			ReadHeaderTimeout: 5 * time.Second,
		},
		logger: logger,
	}
}

// Start runs the server in the background and shuts it down gracefully when ctx
// is cancelled.
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
		s.logger.Info().Str("addr", s.srv.Addr).Msg("Starting health/readiness server")
		if err := s.srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.logger.Error().Err(err).Msg("health server error")
		}
	}()
}
