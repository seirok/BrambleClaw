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
		APIKey:  os.Getenv("BRAMBLE_KEY"),
		BaseURL: os.Getenv("BRAMBLE_URL"),
		Model:   os.Getenv("BRAMBLE_MODEL"),
	}
}
