package testutil

import (
	"brambleclaw/agent"
	"brambleclaw/bus"
	"brambleclaw/channel"
	"brambleclaw/config"
	"brambleclaw/gateway"
	"brambleclaw/sandbox"
	"context"
	"fmt"
	"os"
	"sync"
	"time"
)

type MockChannel struct {
	name         string
	messages     []*bus.OutBoundMessage
	mu           sync.Mutex
	allowedIDs   []string
	enabled      bool
	shouldFail   bool
	OnSendCalled chan *bus.OutBoundMessage
}

func NewMockChannel(name string) *MockChannel {
	return &MockChannel{
		name:         name,
		messages:     make([]*bus.OutBoundMessage, 0),
		allowedIDs:   []string{},
		enabled:      true,
		shouldFail:   false,
		OnSendCalled: make(chan *bus.OutBoundMessage, 10),
	}
}

func (m *MockChannel) Name() string {
	return m.name
}

func (m *MockChannel) Start(ctx context.Context) error {
	return nil
}

func (m *MockChannel) Stop() error {
	return nil
}

func (m *MockChannel) Send(msg *bus.OutBoundMessage) error {
	if m.shouldFail {
		return context.DeadlineExceeded
	}

	m.mu.Lock()
	m.messages = append(m.messages, msg)
	m.mu.Unlock()

	select {
	case m.OnSendCalled <- msg:
	default:
	}

	return nil
}

func (m *MockChannel) IsAllowed(id string) bool {
	if !m.enabled {
		return false
	}
	if len(m.allowedIDs) == 0 {
		return true
	}
	for _, aid := range m.allowedIDs {
		if aid == id {
			return true
		}
	}
	return false
}

func (m *MockChannel) GetMessages() []*bus.OutBoundMessage {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]*bus.OutBoundMessage, len(m.messages))
	copy(result, m.messages)
	return result
}

func (m *MockChannel) ClearMessages() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages = make([]*bus.OutBoundMessage, 0)
}

func CreateTestConfig() *gateway.GatewayConfig {
	return &gateway.GatewayConfig{
		Version:      "1.0",
		DefaultAgent: "main",
		Routes: []gateway.RouteRule{
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
		Retry: gateway.RetryPolicy{
			MaxRetries: 3,
			RetryDelay: 1,
			Timeout:    10,
		},
		HealthCheck: gateway.ChannelHealthCheck{
			Enabled:  false, // 测试中禁用健康检查
			Interval: 30,
			Timeout:  10,
		},
	}
}

func CreateTestSandbox() (*sandbox.Sandbox, string, func(), error) {
	// 创建临时工作目录
	tempDir, err := os.MkdirTemp("", "sandbox_test_*")
	if err != nil {
		fmt.Errorf("创建临时目录失败: %v", err)
		return nil, "", nil, err
	}

	// 创建沙箱配置
	config := &sandbox.SandboxConfig{
		Enabled:          true,
		Workspace:        tempDir,
		AllowReadOutside: false,
		FileSystem: sandbox.FileSystemConfig{
			AllowWritePaths: []string{},
			MaxFileSize:     10 * 1024 * 1024,  // 10MB
			MaxTotalSize:    100 * 1024 * 1024, // 100MB
		},
		Execution: sandbox.ExecutionConfig{
			AllowedCommands: []string{"ls", "cat", "echo", "pwd"},
			Timeout:         5 * time.Second,
			MaxOutputSize:   1024 * 1024, // 1MB
		},
		Audit: sandbox.AuditConfig{
			Enabled: false, // 测试中禁用审计
		},
	}

	sandbox, err := sandbox.NewSandbox(config, nil)
	if err != nil {
		os.RemoveAll(tempDir)
		fmt.Errorf("创建沙箱失败: %v", err)
	}

	cleanup := func() {
		os.RemoveAll(tempDir)
	}

	return sandbox, tempDir, cleanup, nil
}

func CreateTestAgent(name string, llmConfig config.LLMConfig) (*agent.Agent, error) {
	msgBus := bus.NewMessageBus(100)
	agentCfg := agent.AgentConfig{
		Name:       name,
		LLM:        llmConfig,
		MaxHistory: 10,
	}
	testAgent := agent.NewAgent(agentCfg, msgBus, "", 0)

	// 创建并注册沙箱工具
	sandBox, _, _, err := CreateTestSandbox()
	if err != nil {
		return nil, err
	}
	ft := sandbox.NewFileSystemTool(sandBox)
	st := sandbox.NewShellTool(sandBox)
	testAgent.RegisterTool(st)
	testAgent.RegisterTool(ft)

	return testAgent, nil
}

func CreateTestGatewayWithMockChannel() (*gateway.Gateway, *bus.MessageBus, *channel.Manager, *MockChannel) {
	cfg := CreateTestConfig()
	msgBus := bus.NewMessageBus(100)
	channelManager := channel.NewManager(msgBus)

	gateWay := gateway.NewGateway(cfg, msgBus, channelManager)

	// 创建main agent
	conFig, _ := config.Load("../config/config.json")
	llmConfig := conFig.LLMConfig
	mainAgent, _ := CreateTestAgent("main", llmConfig)

	// 注册main agent
	mainCfg := agent.AgentConfig{Name: "main", MaxHistory: 10}
	gateWay.RegisterAgent("main", mainAgent, mainCfg)

	// 创建并注册模拟通道
	mockChannel := NewMockChannel("cli")
	channelManager.Register(mockChannel)

	return gateWay, msgBus, channelManager, mockChannel
}
