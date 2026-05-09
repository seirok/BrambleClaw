package structs

import (
	"brambleclaw/internal/logger"
)

var validLogLevels = map[string]bool{
	"debug": true,
	"info":  true,
	"warn":  true,
	"error": true,
}

// LogConfig 日志配置
type LogConfig struct {
	ConsoleEnabled bool   `json:"console_enabled" mapstructure:"console_enabled"`
	Level          string `json:"level" mapstructure:"level"`
}

// DefaultLogConfig 返回默认日志配置
func DefaultLogConfig() LogConfig {
	return LogConfig{
		ConsoleEnabled: false,
		Level:          "debug",
	}
}

// Validate validates LogConfig and fills defaults.
// Returns whether there was a critical error (never for LogConfig).
func (c *LogConfig) Validate() (hasError bool) {
	defaults := DefaultLogConfig()

	// Note: ConsoleEnabled is a bool - we can't distinguish "unset" from "explicit false"
	// If the user wants the default (true), they must set it explicitly in their config

	// Validate log level
	if c.Level == "" {
		c.Level = defaults.Level
	} else if !validLogLevels[c.Level] {
		logger.L().Warn().Str("invalid_level", c.Level).Msg("Invalid log level, using default")
		c.Level = defaults.Level
	}
	return false
}
