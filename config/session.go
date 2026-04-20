package config

// SessionConfig session 配置
type SessionConfig struct {
	Enabled       bool   `json:"autosave"`       // 是否启用 session 持久化
	StorageFormat string `json:"storage_format"` // 存储格式（目前只支持 jsonl）
	MaxHistory    int    `json:"max_history"`    // 最大历史消息数
}

// DefaultSessionConfig 返回默认 session 配置
func DefaultSessionConfig() SessionConfig {
	return SessionConfig{
		Enabled:       true,
		StorageFormat: "jsonl",
		MaxHistory:    50,
	}
}
