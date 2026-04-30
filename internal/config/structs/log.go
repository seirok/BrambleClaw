package structs

// LogConfig 日志配置
type LogConfig struct {
	ConsoleEnabled bool   `json:"console_enabled" mapstructure:"console_enabled"`
	Level          string `json:"level" mapstructure:"level"`
}

// DefaultLogConfig 返回默认日志配置
func DefaultLogConfig() LogConfig {
	return LogConfig{
		ConsoleEnabled: true,
		Level:          "debug",
	}
}
