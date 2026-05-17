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

// Validate validates AgentConfig and fills defaults.
// Returns whether there was a critical error.
func (c *AgentConfig) Validate() (hasError bool) {
	// Validate name
	if c.Name == "" {
		hasError = true
	}
	ValidateNonEmptyString(&c.Name, "", "Agent name", true)

	// Validate max history
	if c.MaxHistory <= 0 {
		c.MaxHistory = 50
	}
	ValidatePositiveInt(&c.MaxHistory, 50, "max_history")

	// Validate LLM config
	if c.LLM.Validate() {
		// LLM has critical error, but we continue
	}

	// Ensure tools slice is not nil
	EnsureSlice(&c.Tools)
	if len(c.Tools) == 0 {
		c.Tools = []string{"web_search", "shell", "read", "write", "list", "glob", "grep", "url_parse", "grant_permission"}
	}

	return hasError
}
