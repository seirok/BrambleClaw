package structs

import "neoclaw/internal/logger"

// AgentConfig 定义单个 Agent 的配置
type AgentConfig struct {
	Name        string    `json:"name" mapstructure:"name"`               // Agent 名称
	Description string    `json:"description" mapstructure:"description"` // Agent 描述
	LLM         LLMConfig `json:"llm" mapstructure:"llm"`                 // LLM 配置
	Tools       []string  `json:"tools" mapstructure:"tools"`             // Agent 可使用的工具列表
	MaxHistory  int       `json:"max_history" mapstructure:"max_history"` // 最大历史消息数
	Enabled     bool      `json:"enabled" mapstructure:"enabled"`         // 是否启用
}

// Validate validates AgentConfig and fills defaults.
// Returns whether there was a critical error.
func (c *AgentConfig) Validate() (hasError bool) {
	if c.Name == "" {
		logger.L().Error().Msg("Agent name is required")
		hasError = true
	}

	if c.MaxHistory <= 0 {
		logger.L().Warn().Int("invalid_max_history", c.MaxHistory).Msg("Invalid max_history, using 50")
		c.MaxHistory = 50
	}

	// Validate LLM config
	if c.LLM.Validate() {
		// LLM has critical error, but we continue
	}

	if len(c.Tools) == 0 {
		c.Tools = []string{"web_search", "shell", "read", "write", "list", "glob", "grep", "url_parse", "grant_permission"}
	}

	return hasError
}
