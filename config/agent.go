package config

// AgentConfig 定义单个 Agent 的配置
type AgentConfig struct {
	Name        string    `json:"name"`        // Agent 名称
	Description string    `json:"description"` // Agent 描述
	LLM         LLMConfig `json:"llm"`         // LLM 配置
	Tools       []string  `json:"tools"`       // Agent 可使用的工具列表
	Workspace   string    `json:"workspace"`   // Agent 工作目录
	MaxHistory  int       `json:"max_history"` // 最大历史消息数
	Enabled     bool      `json:"enabled"`     // 是否启用
}

// AgentsConfig 管理所有 Agent 配置
type AgentsConfig struct {
	Agents []AgentConfig `json:"agents"` // Agent 配置列表
}

// GetAgent 根据名称获取 Agent 配置
func (ac *AgentsConfig) GetAgent(name string) (*AgentConfig, bool) {
	for i := range ac.Agents {
		if ac.Agents[i].Name == name {
			return &ac.Agents[i], true
		}
	}
	return nil, false
}

// AddOrUpdateAgent 添加或更新 Agent 配置
func (ac *AgentsConfig) AddOrUpdateAgent(config AgentConfig) {
	for i := range ac.Agents {
		if ac.Agents[i].Name == config.Name {
			ac.Agents[i] = config
			return
		}
	}
	ac.Agents = append(ac.Agents, config)
}

// RemoveAgent 删除 Agent 配置
func (ac *AgentsConfig) RemoveAgent(name string) bool {
	for i := range ac.Agents {
		if ac.Agents[i].Name == name {
			ac.Agents = append(ac.Agents[:i], ac.Agents[i+1:]...)
			return true
		}
	}
	return false
}
