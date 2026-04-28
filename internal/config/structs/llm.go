package structs

// LLMConfig LLM 配置
type LLMConfig struct {
	APIKey  string `json:"api_key"`
	BaseURL string `json:"base_url"`
	Model   string `json:"model"`
}

// DefaultLLMConfig 返回默认 LLM 配置
func DefaultLLMConfig() LLMConfig {
	return LLMConfig{
		APIKey:  "",
		BaseURL: "https://api.openai.com/v1",
		Model:   "gpt-3.5-turbo",
	}
}
