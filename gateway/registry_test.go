package gateway

import (
	"fmt"
	"testing"

	"brambleclaw/agent"
	"brambleclaw/bus"
	"brambleclaw/config"
)

// 创建测试用的 Mock Agent
func createMockAgent(t *testing.T) *agent.Agent {
	msgBus := bus.NewMessageBus(100)
	agentCfg := agent.AgentConfig{
		Name: "test-agent",
		LLM: config.LLMConfig{
			APIKey:  "test-key",
			BaseURL: "http://localhost:8080",
			Model:   "test-model",
		},
		MaxHistory: 10,
	}
	return agent.NewAgent(agentCfg, msgBus)
}

func TestNewAgentRegistry(t *testing.T) {
	registry := NewAgentRegistry()
	if registry == nil {
		t.Fatal("NewAgentRegistry() 返回 nil")
	}
	if registry.agents == nil {
		t.Error("agents map 未初始化")
	}
	if registry.Count() != 0 {
		t.Errorf("新注册表应该为空，但有 %d 个 Agent", registry.Count())
	}
}

func TestAgentRegistry_Register(t *testing.T) {
	registry := NewAgentRegistry()
	ag := createMockAgent(t)
	cfg := agent.AgentConfig{Name: "test-agent"}

	// 测试正常注册
	err := registry.Register("test-agent", ag, cfg)
	if err != nil {
		t.Errorf("注册 Agent 失败: %v", err)
	}

	if registry.Count() != 1 {
		t.Errorf("注册后应该只有 1 个 Agent，但有 %d 个", registry.Count())
	}

	// 测试重复注册
	err = registry.Register("test-agent", ag, cfg)
	if err == nil {
		t.Error("重复注册应该返回错误")
	}

	// 测试空名称
	err = registry.Register("", ag, cfg)
	if err == nil {
		t.Error("空名称注册应该返回错误")
	}

	// 测试 nil Agent
	err = registry.Register("nil-agent", nil, cfg)
	if err == nil {
		t.Error("nil Agent 注册应该返回错误")
	}
}

func TestAgentRegistry_Get(t *testing.T) {
	registry := NewAgentRegistry()
	ag := createMockAgent(t)
	cfg := agent.AgentConfig{Name: "test-agent", MaxHistory: 10}

	// 注册 Agent
	err := registry.Register("test-agent", ag, cfg)
	if err != nil {
		t.Fatalf("注册 Agent 失败: %v", err)
	}

	// 测试获取存在的 Agent
	entry, found := registry.Get("test-agent")
	if !found {
		t.Error("应该能找到已注册的 Agent")
	}
	if entry == nil {
		t.Fatal("找到的 Agent entry 不应该为 nil")
	}
	if entry.Name != "test-agent" {
		t.Errorf("Agent 名称应该是 'test-agent'，但得到 '%s'", entry.Name)
	}
	if entry.Config.MaxHistory != 10 {
		t.Errorf("MaxHistory 应该是 10，但得到 %d", entry.Config.MaxHistory)
	}

	// 测试获取不存在的 Agent
	_, found = registry.Get("non-existent")
	if found {
		t.Error("不应该能找到未注册的 Agent")
	}
}

func TestAgentRegistry_GetAgent(t *testing.T) {
	registry := NewAgentRegistry()
	ag := createMockAgent(t)
	cfg := agent.AgentConfig{Name: "test-agent"}

	// 注册 Agent
	registry.Register("test-agent", ag, cfg)

	// 测试获取 Agent 实例
	agentInstance, found := registry.GetAgent("test-agent")
	if !found {
		t.Error("应该能找到 Agent 实例")
	}
	if agentInstance == nil {
		t.Error("Agent 实例不应该为 nil")
	}

	// 测试获取不存在的 Agent
	_, found = registry.GetAgent("non-existent")
	if found {
		t.Error("不应该能找到未注册的 Agent")
	}
}

func TestAgentRegistry_Unregister(t *testing.T) {
	registry := NewAgentRegistry()
	ag := createMockAgent(t)
	cfg := agent.AgentConfig{Name: "test-agent"}

	// 注册 Agent
	registry.Register("test-agent", ag, cfg)

	if registry.Count() != 1 {
		t.Errorf("注册后应该有 1 个 Agent，但有 %d 个", registry.Count())
	}

	// 测试注销存在的 Agent
	success := registry.Unregister("test-agent")
	if !success {
		t.Error("注销已存在的 Agent 应该返回 true")
	}

	if registry.Count() != 0 {
		t.Errorf("注销后应该有 0 个 Agent，但有 %d 个", registry.Count())
	}

	// 测试注销不存在的 Agent
	success = registry.Unregister("non-existent")
	if success {
		t.Error("注销不存在的 Agent 应该返回 false")
	}
}

func TestAgentRegistry_List(t *testing.T) {
	registry := NewAgentRegistry()
	ag := createMockAgent(t)
	cfg := agent.AgentConfig{Name: "test-agent"}

	// 初始状态应该为空
	list := registry.List()
	if len(list) != 0 {
		t.Errorf("初始列表应该为空，但有 %d 个元素", len(list))
	}

	// 注册多个 Agent
	registry.Register("agent1", ag, cfg)
	registry.Register("agent2", ag, cfg)
	registry.Register("agent3", ag, cfg)

	// 验证列表
	list = registry.List()
	if len(list) != 3 {
		t.Errorf("列表应该有 3 个元素，但有 %d 个", len(list))
	}

	// 验证所有 Agent 都在列表中
	agentMap := make(map[string]bool)
	for _, name := range list {
		agentMap[name] = true
	}

	expectedAgents := []string{"agent1", "agent2", "agent3"}
	for _, expected := range expectedAgents {
		if !agentMap[expected] {
			t.Errorf("列表中应该包含 '%s'", expected)
		}
	}
}

func TestAgentRegistry_Count(t *testing.T) {
	registry := NewAgentRegistry()
	ag := createMockAgent(t)
	cfg := agent.AgentConfig{Name: "test-agent"}

	// 初始计数应该为 0
	if registry.Count() != 0 {
		t.Errorf("初始计数应该为 0，但为 %d", registry.Count())
	}

	// 添加 Agent
	registry.Register("agent1", ag, cfg)
	if registry.Count() != 1 {
		t.Errorf("添加 1 个 Agent 后计数应该为 1，但为 %d", registry.Count())
	}

	registry.Register("agent2", ag, cfg)
	if registry.Count() != 2 {
		t.Errorf("添加 2 个 Agent 后计数应该为 2，但为 %d", registry.Count())
	}

	// 删除 Agent
	registry.Unregister("agent1")
	if registry.Count() != 1 {
		t.Errorf("删除 1 个 Agent 后计数应该为 1，但为 %d", registry.Count())
	}
}

func TestAgentRegistry_ConcurrentAccess(t *testing.T) {
	registry := NewAgentRegistry()
	ag := createMockAgent(t)
	cfg := agent.AgentConfig{Name: "test-agent"}

	// 并发注册
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func(index int) {
			name := fmt.Sprintf("agent-%d", index)
			registry.Register(name, ag, cfg)
			done <- true
		}(i)
	}

	// 等待所有 goroutine 完成
	for i := 0; i < 10; i++ {
		<-done
	}

	// 验证所有 Agent 都已注册
	if registry.Count() != 10 {
		t.Errorf("期望 10 个 Agent，但有 %d 个", registry.Count())
	}
}
