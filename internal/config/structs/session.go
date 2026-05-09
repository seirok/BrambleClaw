package structs

import "brambleclaw/internal/logger"

// SessionConfig session 配置
type SessionConfig struct {
	Enabled      bool `json:"autosave" mapstructure:"autosave"`       // 是否启用 session 持久化
	MaxHistory   int  `json:"max_history" mapstructure:"max_history"` // 最大历史消息数
	SaveInterval int  `json:"autosave_interval" mapstructure:"autosave_interval"`
}

// DefaultSessionConfig 返回默认 session 配置
func DefaultSessionConfig() SessionConfig {
	return SessionConfig{
		Enabled:      true,
		MaxHistory:   50,
		SaveInterval: 10,
	}
}

// Validate validates SessionConfig and fills defaults.
// Returns whether there was a critical error (never for SessionConfig).
func (c *SessionConfig) Validate() (hasError bool) {
	defaults := DefaultSessionConfig()

	if c.MaxHistory <= 0 {
		logger.L().Warn().Int("invalid_max_history", c.MaxHistory).Msg("Invalid max_history, using default")
		c.MaxHistory = defaults.MaxHistory
	}

	if c.SaveInterval <= 0 {
		logger.L().Warn().Int("invalid_save_interval", c.SaveInterval).Msg("Invalid save_interval, using default")
		c.SaveInterval = defaults.SaveInterval
	}

	return false
}
