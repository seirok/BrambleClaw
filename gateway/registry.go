package gateway

import (
	"fmt"
	"sync"

	"miniGoClaw/agent"
)

// AgentEntry Agent 注册表项
type AgentEntry struct {
	Name        string
	Agent       *agent.Agent
	Config      agent.AgentConfig
	Description string
}

// AgentRegistry Agent 注册表
// 负责管理所有 Agent 的注册、查询和生命周期管理
type AgentRegistry struct {
	agents map[string]*AgentEntry
	mu     sync.RWMutex
}

// NewAgentRegistry 创建新的 Agent 注册表
func NewAgentRegistry() *AgentRegistry {
	return &AgentRegistry{
		agents: make(map[string]*AgentEntry),
	}
}

// Register 注册 Agent
// 底层职责：参数验证和重复注册检查
func (r *AgentRegistry) Register(name string, ag *agent.Agent, config agent.AgentConfig) error {
	if name == "" {
		return fmt.Errorf("Agent名称不能为空")
	}
	if ag == nil {
		return fmt.Errorf("Agent实例不能为nil(name=%s)", name)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.agents[name]; exists {
		return fmt.Errorf("Agent已存在(name=%s)", name)
	}

	r.agents[name] = &AgentEntry{
		Name:   name,
		Agent:  ag,
		Config: config,
	}

	return nil
}

// Get 获取 Agent
// 中间层职责：仅透传，不包装错误
func (r *AgentRegistry) Get(name string) (*AgentEntry, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	entry, ok := r.agents[name]
	return entry, ok
}

// GetAgent 直接获取 Agent 实例
// 便捷方法，用于快速获取 Agent 进行操作
func (r *AgentRegistry) GetAgent(name string) (*agent.Agent, bool) {
	entry, ok := r.Get(name)
	if !ok {
		return nil, false
	}
	return entry.Agent, true
}

// Unregister 注销 Agent
func (r *AgentRegistry) Unregister(name string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.agents[name]; !exists {
		return false
	}

	delete(r.agents, name)
	return true
}

// List 列出所有已注册的 Agent
func (r *AgentRegistry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.agents))
	for name := range r.agents {
		names = append(names, name)
	}
	return names
}

// Count 返回已注册 Agent 数量
func (r *AgentRegistry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return len(r.agents)
}
