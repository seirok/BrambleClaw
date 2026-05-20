package httpserver

import "strings"

// ChatRequest is the request body for POST /api/chat
type ChatRequest struct {
	Message   string `json:"message"`
	AgentName string `json:"agent_name,omitempty"`
	ChatID    string `json:"chat_id,omitempty"`
}

// ConfigResponse is the redacted config response for admin API
type ConfigResponse struct {
	Log        any `json:"log"`
	BusBufSize int `json:"bus-buf-size"`
	SubBufSize int `json:"sub-buf-size"`
	Channels   any `json:"channels"`
	LLMConfig  any `json:"llm"`
	Tools      any `json:"tools"`
	Gateway    any `json:"gateway"`
	Agents     any `json:"agents"`
	Session    any `json:"session"`
	Compact    any `json:"compact"`
	Sandbox    any `json:"sandbox"`
	Hooks      any `json:"hooks"`
	Sidebar    any `json:"sidebar"`
	Skill      any `json:"skill"`
	Web        any `json:"web"`
}

// AgentInfo is the response body for GET /api/admin/agents
type AgentInfo struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Model       string   `json:"model"`
	Status      string   `json:"status"`
	Tools       []string `json:"tools"`
}

// HealthResponse is the response body for GET /api/health
type HealthResponse struct {
	Status string `json:"status"`
	Time   string `json:"time"`
}

// RedactValue returns "redacted" for non-empty values
func RedactValue(key string, val any) any {
	if val != nil && val != "" {
		return "***redacted***"
	}
	return val
}

// IsSecretField checks if a field name suggests it holds a secret
func IsSecretField(key string) bool {
	lower := strings.ToLower(key)
	return strings.Contains(lower, "key") ||
		strings.Contains(lower, "secret") ||
		strings.Contains(lower, "token") ||
		strings.Contains(lower, "password") ||
		strings.Contains(lower, "aes")
}
