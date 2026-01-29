package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/netspec/netspec/internal/alerter"
	"github.com/rs/zerolog"
)

// Server provides HTTP API endpoints
type Server struct {
	alertEngine *alerter.Engine
	logger      zerolog.Logger
	port        string
}

// NewServer creates a new API server
func NewServer(alertEngine *alerter.Engine, logger zerolog.Logger, port string) *Server {
	return &Server{
		alertEngine: alertEngine,
		logger:      logger,
		port:        port,
	}
}

// Start starts the HTTP server
func (s *Server) Start() error {
	mux := http.NewServeMux()
	
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/status", s.handleStatus)
	mux.HandleFunc("/alerts", s.handleAlerts)

	addr := ":" + s.port
	s.logger.Info().
		Str("address", addr).
		Msg("Starting API server")

	return http.ListenAndServe(addr, mux)
}

// handleHealth returns service health status
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status": "healthy",
		"time":   time.Now().UTC().Format(time.RFC3339),
	})
}

// handleStatus returns current state summary
func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	
	alerts := s.alertEngine.GetActiveAlerts()
	status := map[string]interface{}{
		"active_alerts": len(alerts),
		"time":          time.Now().UTC().Format(time.RFC3339),
	}
	
	json.NewEncoder(w).Encode(status)
}

// handleAlerts returns active alerts
func (s *Server) handleAlerts(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	
	alerts := s.alertEngine.GetActiveAlerts()
	json.NewEncoder(w).Encode(map[string]interface{}{
		"alerts": alerts,
		"count":  len(alerts),
	})
}
