package structs

// LogConfig 日志配置
type LogConfig struct {
	Path           string `json:"path"`
	ConsoleEnabled bool   `json:"console_enabled"`
	Level          string `json:"level"`
}

// DefaultLogConfig 返回默认日志配置
func DefaultLogConfig() LogConfig {
	return LogConfig{
		Path:           "logs/brambleclaw.log",
		ConsoleEnabled: true,
		Level:          "debug",
	}
}
