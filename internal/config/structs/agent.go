package structs

// AgentConfig 定义单个 Agent 的配置
type AgentConfig struct {
	Name        string    `json:"name" mapstructure:"name"`               // Agent 名称
	Description string    `json:"description" mapstructure:"description"` // Agent 描述
	LLM         LLMConfig `json:"llm" mapstructure:"llm"`                 // LLM 配置
	Tools       []string  `json:"tools" mapstructure:"tools"`             // Agent 可使用的工具列表
	MaxHistory  int       `json:"max_history" mapstructure:"max_history"` // 最大历史消息数
	Enabled     bool      `json:"enabled" mapstructure:"enabled"`         // 是否启用
}
