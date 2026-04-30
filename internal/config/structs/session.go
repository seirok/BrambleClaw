package structs

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
