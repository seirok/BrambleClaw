package httpserver

import (
	"net/http"
	"time"
)

// handleMetricsSummary returns aggregated metrics summary
func (s *Server) handleMetricsSummary(w http.ResponseWriter, r *http.Request) {
	if !s.checkAPIKey(w, r) {
		return
	}

	summary := s.metrics.GetSummary()
	summary["updated_at"] = time.Now().UTC().Format(time.RFC3339)
	s.jsonResponse(w, http.StatusOK, summary)
}

// handleMetricsChannels returns per-endpoint metrics
func (s *Server) handleMetricsChannels(w http.ResponseWriter, r *http.Request) {
	if !s.checkAPIKey(w, r) {
		return
	}

	perEndpoint := s.metrics.GetPerEndpoint()
	s.jsonResponse(w, http.StatusOK, perEndpoint)
}
