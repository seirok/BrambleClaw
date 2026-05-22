package httpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"neoclaw/internal/agent"
	"neoclaw/internal/bus"
	"neoclaw/internal/config"
	"neoclaw/internal/logger"
)

// Server is the HTTP server for web frontend
type Server struct {
	srv      *http.Server
	msgBus   *bus.MessageBus
	agentMgr *agent.AgentManager
	metrics  *MetricsCollector
	webDir   string // path to web/ static files
}

// NewServer creates a new HTTP server
func NewServer(msgBus *bus.MessageBus, agentMgr *agent.AgentManager) *Server {
	cfg := config.Get().Web
	s := &Server{
		msgBus:   msgBus,
		agentMgr: agentMgr,
		metrics:  NewMetricsCollector(),
		webDir:   findWebDir(),
	}

	router := http.NewServeMux()

	// API routes (must be registered first)
	router.HandleFunc("GET /api/health", s.handleHealth)
	router.HandleFunc("POST /api/chat", s.handleChat)
	router.HandleFunc("POST /api/chat/reset", s.handleChatReset)
	router.HandleFunc("GET /api/admin/config", s.handleGetConfig)
	router.HandleFunc("PUT /api/admin/config", s.handleUpdateConfig)
	router.HandleFunc("GET /api/admin/agents", s.handleListAgents)
	router.HandleFunc("POST /api/admin/agents/reset", s.handleResetAgentSession)
	router.HandleFunc("GET /api/admin/audit", s.handleGetAuditLog)
	router.HandleFunc("GET /api/metrics/summary", s.handleMetricsSummary)
	router.HandleFunc("GET /api/metrics/channels", s.handleMetricsChannels)
	router.HandleFunc("POST /api/log", s.handleClientLog)

	// Static files + SPA fallback
	handler := s.spaHandler(s.webDir, router)
	handler = s.corsMiddleware(handler)
	handler = s.metricsMiddleware(handler)

	s.srv = &http.Server{
		Addr:         fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
		Handler:      handler,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 0, // SSE needs unlimited write timeout
		IdleTimeout:  120 * time.Second,
	}

	return s
}

// Start starts the HTTP server
func (s *Server) Start(ctx context.Context) error {
	cfg := config.Get().Web
	if !cfg.Enabled {
		return nil
	}

	go func() {
		logger.L().Info().Str("addr", s.srv.Addr).Str("web_dir", s.webDir).Msg("HTTP server starting")
		if err := s.srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.L().Error().Err(err).Msg("HTTP server error")
		}
	}()

	return nil
}

// Stop gracefully stops the HTTP server
func (s *Server) Stop(ctx context.Context) error {
	logger.L().Info().Msg("HTTP server stopping")
	return s.srv.Shutdown(ctx)
}

// handleHealth returns server health status
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	s.jsonResponse(w, http.StatusOK, map[string]interface{}{
		"status": "ok",
		"time":   time.Now().UTC().Format(time.RFC3339),
	})
}

// spaHandler serves static files and falls back to index.html for SPA routing
func (s *Server) spaHandler(webDir string, apiRouter http.Handler) http.Handler {
	fileServer := http.FileServer(http.Dir(webDir))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// API routes go to the API router
		if r.URL.Path == "/api/health" ||
			r.URL.Path == "/api/chat" ||
			r.URL.Path == "/api/chat/reset" ||
			r.URL.Path == "/api/admin/config" ||
			r.URL.Path == "/api/admin/agents" ||
			r.URL.Path == "/api/admin/agents/reset" ||
			r.URL.Path == "/api/admin/audit" ||
			r.URL.Path == "/api/metrics/summary" ||
			r.URL.Path == "/api/metrics/channels" ||
			r.URL.Path == "/api/log" {
			apiRouter.ServeHTTP(w, r)
			return
		}

		// Try to serve static file
		fullPath := filepath.Join(webDir, filepath.Clean(r.URL.Path))
		if info, err := os.Stat(fullPath); err == nil && !info.IsDir() {
			fileServer.ServeHTTP(w, r)
			return
		}

		// SPA fallback: serve index.html for non-file routes
		indexFile := filepath.Join(webDir, "index.html")
		if _, err := os.Stat(indexFile); err == nil {
			http.ServeFile(w, r, indexFile)
			return
		}

		http.NotFound(w, r)
	})
}

// findWebDir returns the path to the web/ directory
func findWebDir() string {
	// Try common locations
	candidates := []string{
		"web/dist", // Production: Vite build output
		"web",      // Development: source files
		"./web/dist",
		"./web",
		filepath.Join("cmd", "neoclaw", "web", "dist"),
		filepath.Join("cmd", "neoclaw", "web"),
	}

	for _, dir := range candidates {
		full := filepath.Join(filepath.Dir(os.Args[0]), dir)
		if info, err := os.Stat(full); err == nil && info.IsDir() {
			return full
		}
		// Also try relative to current working directory
		full = dir
		if info, err := os.Stat(full); err == nil && info.IsDir() {
			return full
		}
	}

	// Fallback: current directory
	return "web"
}

// corsMiddleware handles CORS headers
func (s *Server) corsMiddleware(next http.Handler) http.Handler {
	cfg := config.Get().Web
	originSet := make(map[string]bool)
	for _, origin := range cfg.AllowedOrigins {
		originSet[origin] = true
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if originSet[origin] || len(cfg.AllowedOrigins) == 0 {
			w.Header().Set("Access-Control-Allow-Origin", origin)
		}
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-API-Key")
		w.Header().Set("Access-Control-Allow-Credentials", "true")
		w.Header().Set("Access-Control-Max-Age", "86400")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// metricsMiddleware records request metrics
func (s *Server) metricsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		next.ServeHTTP(w, r)

		duration := time.Since(start)
		hasError := false // TODO: capture from response status code
		s.metrics.RecordRequest(r.URL.Path, duration, hasError)
	})
}

// jsonResponse writes a JSON response
func (s *Server) jsonResponse(w http.ResponseWriter, code int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		logger.L().Error().Err(err).Msg("Failed to encode JSON response")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}
