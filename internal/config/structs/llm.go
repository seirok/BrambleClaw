package structs

import "os"

// LLMConfig LLM 配置
type LLMConfig struct {
	APIKey  string
	BaseURL string
	Model   string
}

// DefaultLLMConfig 返回默认 LLM 配置
func DefaultLLMConfig() LLMConfig {
	return LLMConfig{
		APIKey:  os.Getenv("NEO_KEY"),
		BaseURL: os.Getenv("NEO_URL"),
		Model:   os.Getenv("NEO_MODEL"),
	}
}

// Validate validates LLMConfig and fills defaults.
// Returns whether there was a critical error.
func (c *LLMConfig) Validate() (hasError bool) {
	defaults := DefaultLLMConfig()

	// Check APIKey with error log
	if c.APIKey == "" {
		hasError = true
	}
	ValidateNonEmptyString(&c.APIKey, defaults.APIKey, "LLM APIKey", true)

	ValidateNonEmptyString(&c.BaseURL, defaults.BaseURL, "LLM BaseURL", false)
	ValidateNonEmptyString(&c.Model, defaults.Model, "LLM Model", false)

	return hasError
}
