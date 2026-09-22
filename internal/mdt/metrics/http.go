package metrics

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"time"

	"github.com/netspec/netspec/internal/mdt/egress"
	"github.com/netspec/netspec/internal/mdt/receiver"
	"github.com/rs/zerolog"
)

// Snapshot is the sidecar /stats payload.
type Snapshot struct {
	Status   string         `json:"status"`
	Time     time.Time      `json:"time"`
	Version  string         `json:"version"`
	Uptime   string         `json:"uptime"`
	Receiver receiver.Stats `json:"receiver"`
	Forward  egress.Stats   `json:"forward"`
}

// Gatherer supplies a live snapshot for HTTP handlers.
type Gatherer func() Snapshot

// Server serves /health, /stats (JSON), and /metrics (Prometheus text).
type Server struct {
	gather Gatherer
	log    zerolog.Logger
	http   *http.Server
}

func NewServer(addr string, gather Gatherer, log zerolog.Logger) *Server {
	if gather == nil {
		gather = func() Snapshot { return Snapshot{Status: "healthy", Time: time.Now().UTC()} }
	}
	s := &Server{gather: gather, log: log}
	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/stats", s.handleStats)
	mux.HandleFunc("/metrics", s.handleMetrics)
	s.http = &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	return s
}

func (s *Server) Serve(ctx context.Context, ln net.Listener) error {
	errCh := make(chan error, 1)
	go func() {
		s.log.Info().Str("address", ln.Addr().String()).Msg("MDT metrics HTTP listening")
		errCh <- s.http.Serve(ln)
	}()
	select {
	case <-ctx.Done():
		shutCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = s.http.Shutdown(shutCtx)
		<-errCh
		return nil
	case err := <-errCh:
		if err != nil && err != http.ErrServerClosed {
			return err
		}
		return nil
	}
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{
		"status": "healthy",
		"time":   time.Now().UTC().Format(time.RFC3339),
	})
}

func (s *Server) handleStats(w http.ResponseWriter, _ *http.Request) {
	snap := s.gather()
	if snap.Status == "" {
		snap.Status = "healthy"
	}
	if snap.Time.IsZero() {
		snap.Time = time.Now().UTC()
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(snap)
}

func (s *Server) handleMetrics(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	_ = WritePrometheus(w, s.gather())
}
