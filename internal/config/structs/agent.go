package structs

// AgentConfig 定义单个 Agent 的配置
type AgentConfig struct {
	Name        string    `json:"name"`        // Agent 名称
	Description string    `json:"description"` // Agent 描述
	LLM         LLMConfig `json:"llm"`         // LLM 配置
	Tools       []string  `json:"tools"`       // Agent 可使用的工具列表
	MaxHistory  int       `json:"max_history"` // 最大历史消息数
	Enabled     bool      `json:"enabled"`     // 是否启用
}
