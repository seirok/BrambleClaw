package httpserver

import (
	"encoding/json"
	"net/http"

	util "neoclaw/internal"
	"neoclaw/internal/config"
	"neoclaw/internal/logger"
)

// handleGetConfig returns the current config with secrets redacted
func (s *Server) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	if !s.checkAPIKey(w, r) {
		return
	}

	cfg := config.Get()
	// Convert to map and redact secrets
	var cfgMap map[string]any
	b, _ := json.Marshal(cfg)
	json.Unmarshal(b, &cfgMap)

	redacted := redactMap(cfgMap)
	s.jsonResponse(w, http.StatusOK, redacted)
}

// handleUpdateConfig updates and persists the config
func (s *Server) handleUpdateConfig(w http.ResponseWriter, r *http.Request) {
	if !s.checkAPIKey(w, r) {
		return
	}

	var newCfg config.Config
	if err := json.NewDecoder(r.Body).Decode(&newCfg); err != nil {
		s.jsonResponse(w, http.StatusBadRequest, map[string]string{
			"error": "invalid request body",
		})
		return
	}

	// Validate the new config
	if newCfg.Web.APIKey == "" {
		newCfg.Web.APIKey = config.Get().Web.APIKey
	}

	// Save to disk
	cfgPath := util.GetGlobalConfigPath()
	if err := util.SaveStructToJSON(cfgPath, &newCfg); err != nil {
		s.jsonResponse(w, http.StatusInternalServerError, map[string]string{
			"error": "failed to save config",
		})
		return
	}

	logger.L().Info().Msg("Config updated via web API")
	s.jsonResponse(w, http.StatusOK, map[string]string{
		"status":  "ok",
		"message": "config updated successfully",
	})
}

// handleListAgents returns all agents with their status
func (s *Server) handleListAgents(w http.ResponseWriter, r *http.Request) {
	if !s.checkAPIKey(w, r) {
		return
	}

	cfg := config.Get()
	agents := make([]AgentInfo, 0, len(cfg.Agents))
	for _, a := range cfg.Agents {
		agents = append(agents, AgentInfo{
			Name:        a.Name,
			Description: a.Description,
			Model:       a.LLM.Model,
			Status:      "active",
			Tools:       a.Tools,
		})
	}

	s.jsonResponse(w, http.StatusOK, agents)
}

// handleResetAgentSession resets an agent's session
func (s *Server) handleResetAgentSession(w http.ResponseWriter, r *http.Request) {
	if !s.checkAPIKey(w, r) {
		return
	}

	// TODO: implement session reset
	s.jsonResponse(w, http.StatusOK, map[string]string{
		"status":  "ok",
		"message": "agent session reset endpoint (TODO)",
	})
}

// handleGetAuditLog returns recent audit log entries
func (s *Server) handleGetAuditLog(w http.ResponseWriter, r *http.Request) {
	if !s.checkAPIKey(w, r) {
		return
	}

	s.jsonResponse(w, http.StatusOK, []any{})
}

// checkAPIKey validates the API key from request headers
func (s *Server) checkAPIKey(w http.ResponseWriter, r *http.Request) bool {
	cfg := config.Get().Web
	if cfg.APIKey == "" {
		return true
	}

	key := r.Header.Get("X-API-Key")
	if key == "" {
		key = r.Header.Get("Authorization")
		if len(key) > 7 && key[:7] == "Bearer " {
			key = key[7:]
		}
	}

	if key != cfg.APIKey {
		s.jsonResponse(w, http.StatusUnauthorized, map[string]string{
			"error": "unauthorized",
		})
		return false
	}

	return true
}

// redactMap recursively redacts secret fields from a map
func redactMap(m map[string]any) map[string]any {
	result := make(map[string]any)
	for key, val := range m {
		if IsSecretField(key) {
			result[key] = "***redacted***"
		} else if nested, ok := val.(map[string]any); ok {
			result[key] = redactMap(nested)
		} else {
			result[key] = val
		}
	}
	return result
}
