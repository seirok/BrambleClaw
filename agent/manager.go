package agent

import (
	"context"
	"errors"
	"log"
	"miniGoClaw/bus"
	"sync"
)

// AgentManager Agent管理器
type AgentManager struct {
	agents map[string]*Agent
	bus    *bus.MessageBus
	mu     sync.RWMutex
}

// NewAgentManager 创建Agent管理器
func NewAgentManager(bus *bus.MessageBus) *AgentManager {
	return &AgentManager{
		agents: make(map[string]*Agent),
		bus:    bus,
		mu:     sync.RWMutex{},
	}
}

// Register 注册Agent
func (m *AgentManager) Register(name string, agent *Agent) error {
	if _, ok := m.agents[name]; ok {
		log.Printf("Agent %s already registered", name)
		return errors.New("Agent " + name + " already registered")
	}
	m.agents[name] = agent
	return nil
}

// Start 启动所有Agent
func (m *AgentManager) Start(ctx context.Context) error {
	for _, agent := range m.agents {
		if err := agent.Start(ctx); err != nil {
			log.Printf("Agent %s fail to start", agent.config.Name)
			continue
		}
	}
	return nil
}

// Stop 停止所有Agent
func (m *AgentManager) Stop() {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, agent := range m.agents {
		agent.Stop()
	}
}

// Get 获取Agent
func (m *AgentManager) Get(name string) (*Agent, bool) {
	if _, ok := m.agents[name]; ok {
		return m.agents[name], true
	}
	log.Printf("Agent %s not registered", name)
	return nil, false
}
