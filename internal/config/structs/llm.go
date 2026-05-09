package structs

import (
	"os"

	"brambleclaw/internal/logger"
)

// LLMConfig LLM 配置
type LLMConfig struct {
	APIKey  string
	BaseURL string
	Model   string
}

// DefaultLLMConfig 返回默认 LLM 配置
func DefaultLLMConfig() LLMConfig {
	return LLMConfig{
		APIKey:  os.Getenv("BRAMBLE_KEY"),
		BaseURL: os.Getenv("BRAMBLE_URL"),
		Model:   os.Getenv("BRAMBLE_MODEL"),
	}
}

// Validate validates LLMConfig and fills defaults.
// Returns whether there was a critical error.
func (c *LLMConfig) Validate() (hasError bool) {
	defaults := DefaultLLMConfig()

	if c.APIKey == "" {
		logger.L().Error().Msg("LLM APIKey is required")
		hasError = true
		c.APIKey = defaults.APIKey
	}

	if c.BaseURL == "" {
		c.BaseURL = defaults.BaseURL
	}

	if c.Model == "" {
		c.Model = defaults.Model
	}

	return hasError
}
