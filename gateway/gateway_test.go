package gateway

import (
	"context"
	"fmt"
	"testing"
	"time"

	"brambleclaw/agent"
	"brambleclaw/bus"
	"brambleclaw/channel"
	"brambleclaw/config"
)

// 创建测试用的 Gateway 配置
func createTestConfig(t *testing.T) *GatewayConfig {
	return &GatewayConfig{
		Version:      "1.0",
		DefaultAgent: "main",
		Routes: []RouteRule{
			{
				Channel:  "cli",
				Agent:    "main",
				Priority: 10,
			},
			{
				Channel:  "weixin",
				Agent:    "customer_service",
				Priority: 20,
			},
		},
		Retry: RetryPolicy{
			MaxRetries: 3,
			RetryDelay: 1,
			Timeout:    10,
		},
		HealthCheck: ChannelHealthCheck{
			Enabled:  false, // 测试中禁用健康检查
			Interval: 30,
			Timeout:  10,
		},
	}
}

// 创建测试用的 Agent
func createTestAgent(t *testing.T, name string, llmConfig config.LLMConfig) *agent.Agent {
	msgBus := bus.NewMessageBus(100)
	agentCfg := agent.AgentConfig{
		Name:       name,
		LLM:        llmConfig,
		MaxHistory: 10,
	}
	return agent.NewAgent(agentCfg, msgBus)
}

// 创建测试用的 Gateway
func createTestGateway(t *testing.T) (*Gateway, *bus.MessageBus, *channel.Manager) {
	cfg := createTestConfig(t)
	msgBus := bus.NewMessageBus(100)
	channelManager := channel.NewManager(msgBus)

	gateway := NewGateway(cfg, msgBus, channelManager)

	// 注册测试 Agent
	config, _ := config.Load("../config/config.json")
	llmConfig := config.LLMConfig
	mainAgent := createTestAgent(t, "main", llmConfig)
	mainCfg := agent.AgentConfig{Name: "main", MaxHistory: 10}
	gateway.RegisterAgent("main", mainAgent, mainCfg)

	csAgent := createTestAgent(t, "customer_service", llmConfig)
	csCfg := agent.AgentConfig{Name: "customer_service", MaxHistory: 10}
	gateway.RegisterAgent("customer_service", csAgent, csCfg)

	return gateway, msgBus, channelManager
}

func TestNewGateway(t *testing.T) {
	cfg := createTestConfig(t)
	msgBus := bus.NewMessageBus(100)
	channelManager := channel.NewManager(msgBus)

	gateway := NewGateway(cfg, msgBus, channelManager)

	if gateway == nil {
		t.Fatal("NewGateway() 返回 nil")
	}

	if gateway.config != cfg {
		t.Error("Gateway config 不匹配")
	}

	if gateway.msgBus != msgBus {
		t.Error("Gateway msgBus 不匹配")
	}

	if gateway.channelManager != channelManager {
		t.Error("Gateway channelManager 不匹配")
	}

	if gateway.registry == nil {
		t.Error("Gateway registry 未初始化")
	}

	if gateway.router == nil {
		t.Error("Gateway router 未初始化")
	}

	if gateway.running {
		t.Error("新创建的 Gateway 不应该处于运行状态")
	}
}

func TestGateway_RegisterAgent(t *testing.T) {
	gateway, _, _ := createTestGateway(t)

	// 测试注册新 Agent
	config, _ := config.Load("../config/config.json")
	llmConfig := config.LLMConfig
	newAgent := createTestAgent(t, "new-agent", llmConfig)
	newCfg := agent.AgentConfig{Name: "new-agent"}

	err := gateway.RegisterAgent("new-agent", newAgent, newCfg)
	if err != nil {
		t.Errorf("注册新 Agent 失败: %v", err)
	}

	if gateway.registry.Count() != 3 { // 2个测试Agent + 1个新Agent
		t.Errorf("注册后应该有 3 个 Agent，但有 %d 个", gateway.registry.Count())
	}

	// 测试重复注册
	err = gateway.RegisterAgent("new-agent", newAgent, newCfg)
	if err == nil {
		t.Error("重复注册应该返回错误")
	}
}

func TestGateway_StartStop(t *testing.T) {
	gateway, _, _ := createTestGateway(t)
	ctx := context.Background()

	// 测试启动
	err := gateway.Start(ctx)
	if err != nil {
		t.Fatalf("启动 Gateway 失败: %v", err)
	}

	if !gateway.IsRunning() {
		t.Error("Gateway 应该处于运行状态")
	}

	// 测试重复启动
	err = gateway.Start(ctx)
	if err == nil {
		t.Error("重复启动应该返回错误")
	}

	// 测试停止
	err = gateway.Stop()
	if err != nil {
		t.Errorf("停止 Gateway 失败: %v", err)
	}

	// 给一些时间让 goroutine 结束
	time.Sleep(100 * time.Millisecond)

	if gateway.IsRunning() {
		t.Error("Gateway 应该已停止")
	}

	// 测试重复停止
	err = gateway.Stop()
	if err != nil {
		t.Errorf("重复停止不应该返回错误: %v", err)
	}
}

func TestGateway_GetRegistry(t *testing.T) {
	gateway, _, _ := createTestGateway(t)

	registry := gateway.GetRegistry()
	if registry == nil {
		t.Error("GetRegistry() 不应该返回 nil")
	}

	// 验证注册表中的内容
	if registry.Count() != 2 {
		t.Errorf("注册表中应该有 2 个 Agent，但有 %d 个", registry.Count())
	}
}

func TestGateway_GetConfig(t *testing.T) {
	gateway, _, _ := createTestGateway(t)

	cfg := gateway.GetConfig()
	if cfg == nil {
		t.Fatal("GetConfig() 不应该返回 nil")
	}

	if cfg.Version != "1.0" {
		t.Errorf("Version 期望 '1.0'，得到 '%s'", cfg.Version)
	}

	if cfg.DefaultAgent != "main" {
		t.Errorf("DefaultAgent 期望 'main'，得到 '%s'", cfg.DefaultAgent)
	}
}

func TestGateway_ConcurrentAccess(t *testing.T) {
	gateway, _, _ := createTestGateway(t)
	ctx := context.Background()

	// 启动 Gateway
	if err := gateway.Start(ctx); err != nil {
		t.Fatalf("启动 Gateway 失败: %v", err)
	}

	defer gateway.Stop()

	// 并发注册 Agent
	done := make(chan bool, 5)
	config, _ := config.Load("../config/config.json")
	llmConfig := config.LLMConfig
	for i := 0; i < 5; i++ {
		go func(index int) {
			name := fmt.Sprintf("concurrent-agent-%d", index)
			testAgent := createTestAgent(t, name, llmConfig)
			cfg := agent.AgentConfig{Name: name}
			gateway.RegisterAgent(name, testAgent, cfg)
			done <- true
		}(i)
	}

	// 等待所有 goroutine 完成
	for i := 0; i < 5; i++ {
		<-done
	}

	// 验证所有 Agent 都已注册
	expectedCount := 7 // 2个初始 + 5个并发
	if gateway.registry.Count() != expectedCount {
		t.Errorf("期望 %d 个 Agent，但有 %d 个", expectedCount, gateway.registry.Count())
	}
}
