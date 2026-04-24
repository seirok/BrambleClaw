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

// 鍒涘缓娴嬭瘯鐢ㄧ殑 Gateway 閰嶇疆
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
			Enabled:  false, // 娴嬭瘯涓鐢ㄥ仴搴锋鏌?			Interval: 30,
			Timeout:  10,
		},
	}
}

// 鍒涘缓娴嬭瘯鐢ㄧ殑 Agent
func createTestAgent(t *testing.T, name string, llmConfig config.LLMConfig) *agent.Agent {
	msgBus := bus.NewMessageBus(100)
	agentCfg := config.AgentConfig{
		Name:       name,
		LLM:        llmConfig,
		MaxHistory: 10,
	}
	return agent.NewAgent(agentCfg, msgBus)
}

// 鍒涘缓娴嬭瘯鐢ㄧ殑 Gateway
func createTestGateway(t *testing.T) (*Gateway, *bus.MessageBus, *channel.Manager) {
	cfg := createTestConfig(t)
	msgBus := bus.NewMessageBus(100)
	channelManager := channel.NewManager(msgBus)

	gateway := NewGateway(cfg, msgBus, channelManager)

	// 娉ㄥ唽娴嬭瘯 Agent
	config, _ := config.Load("../config/config.json")
	llmConfig := config.LLMConfig
	mainAgent := createTestAgent(t, "main", llmConfig)
	mainCfg := config.AgentConfig{Name: "main", MaxHistory: 10}
	gateway.RegisterAgent("main", mainAgent, mainCfg)

	csAgent := createTestAgent(t, "customer_service", llmConfig)
	csCfg := config.AgentConfig{Name: "customer_service", MaxHistory: 10}
	gateway.RegisterAgent("customer_service", csAgent, csCfg)

	return gateway, msgBus, channelManager
}

func TestNewGateway(t *testing.T) {
	cfg := createTestConfig(t)
	msgBus := bus.NewMessageBus(100)
	channelManager := channel.NewManager(msgBus)

	gateway := NewGateway(cfg, msgBus, channelManager)

	if gateway == nil {
		t.Fatal("NewGateway() 杩斿洖 nil")
	}

	if gateway.config != cfg {
		t.Error("Gateway config 涓嶅尮閰?)
	}

	if gateway.msgBus != msgBus {
		t.Error("Gateway msgBus 涓嶅尮閰?)
	}

	if gateway.channelManager != channelManager {
		t.Error("Gateway channelManager 涓嶅尮閰?)
	}

	if gateway.registry == nil {
		t.Error("Gateway registry 鏈垵濮嬪寲")
	}

	if gateway.router == nil {
		t.Error("Gateway router 鏈垵濮嬪寲")
	}

	if gateway.running {
		t.Error("鏂板垱寤虹殑 Gateway 涓嶅簲璇ュ浜庤繍琛岀姸鎬?)
	}
}

func TestGateway_RegisterAgent(t *testing.T) {
	gateway, _, _ := createTestGateway(t)

	// 娴嬭瘯娉ㄥ唽鏂?Agent
	config, _ := config.Load("../config/config.json")
	llmConfig := config.LLMConfig
	newAgent := createTestAgent(t, "new-agent", llmConfig)
	newCfg := config.AgentConfig{Name: "new-agent"}

	err := gateway.RegisterAgent("new-agent", newAgent, newCfg)
	if err != nil {
		t.Errorf("娉ㄥ唽鏂?Agent 澶辫触: %v", err)
	}

	if gateway.registry.Count() != 3 { // 2涓祴璇旳gent + 1涓柊Agent
		t.Errorf("娉ㄥ唽鍚庡簲璇ユ湁 3 涓?Agent锛屼絾鏈?%d 涓?, gateway.registry.Count())
	}

	// 娴嬭瘯閲嶅娉ㄥ唽
	err = gateway.RegisterAgent("new-agent", newAgent, newCfg)
	if err == nil {
		t.Error("閲嶅娉ㄥ唽搴旇杩斿洖閿欒")
	}
}

func TestGateway_StartStop(t *testing.T) {
	gateway, _, _ := createTestGateway(t)
	ctx := context.Background()

	// 娴嬭瘯鍚姩
	err := gateway.Start(ctx)
	if err != nil {
		t.Fatalf("鍚姩 Gateway 澶辫触: %v", err)
	}

	if !gateway.IsRunning() {
		t.Error("Gateway 搴旇澶勪簬杩愯鐘舵€?)
	}

	// 娴嬭瘯閲嶅鍚姩
	err = gateway.Start(ctx)
	if err == nil {
		t.Error("閲嶅鍚姩搴旇杩斿洖閿欒")
	}

	// 娴嬭瘯鍋滄
	err = gateway.Stop()
	if err != nil {
		t.Errorf("鍋滄 Gateway 澶辫触: %v", err)
	}

	// 缁欎竴浜涙椂闂磋 goroutine 缁撴潫
	time.Sleep(100 * time.Millisecond)

	if gateway.IsRunning() {
		t.Error("Gateway 搴旇宸插仠姝?)
	}

	// 娴嬭瘯閲嶅鍋滄
	err = gateway.Stop()
	if err != nil {
		t.Errorf("閲嶅鍋滄涓嶅簲璇ヨ繑鍥為敊璇? %v", err)
	}
}

func TestGateway_GetRegistry(t *testing.T) {
	gateway, _, _ := createTestGateway(t)

	registry := gateway.GetRegistry()
	if registry == nil {
		t.Error("GetRegistry() 涓嶅簲璇ヨ繑鍥?nil")
	}

	// 楠岃瘉娉ㄥ唽琛ㄤ腑鐨勫唴瀹?	if registry.Count() != 2 {
		t.Errorf("娉ㄥ唽琛ㄤ腑搴旇鏈?2 涓?Agent锛屼絾鏈?%d 涓?, registry.Count())
	}
}

func TestGateway_GetConfig(t *testing.T) {
	gateway, _, _ := createTestGateway(t)

	cfg := gateway.GetConfig()
	if cfg == nil {
		t.Fatal("GetConfig() 涓嶅簲璇ヨ繑鍥?nil")
	}

	if cfg.Version != "1.0" {
		t.Errorf("Version 鏈熸湜 '1.0'锛屽緱鍒?'%s'", cfg.Version)
	}

	if cfg.DefaultAgent != "main" {
		t.Errorf("DefaultAgent 鏈熸湜 'main'锛屽緱鍒?'%s'", cfg.DefaultAgent)
	}
}

func TestGateway_ConcurrentAccess(t *testing.T) {
	gateway, _, _ := createTestGateway(t)
	ctx := context.Background()

	// 鍚姩 Gateway
	if err := gateway.Start(ctx); err != nil {
		t.Fatalf("鍚姩 Gateway 澶辫触: %v", err)
	}

	defer gateway.Stop()

	// 骞跺彂娉ㄥ唽 Agent
	done := make(chan bool, 5)
	config, _ := config.Load("../config/config.json")
	llmConfig := config.LLMConfig
	for i := 0; i < 5; i++ {
		go func(index int) {
			name := fmt.Sprintf("concurrent-agent-%d", index)
			testAgent := createTestAgent(t, name, llmConfig)
			cfg := config.AgentConfig{Name: name}
			gateway.RegisterAgent(name, testAgent, cfg)
			done <- true
		}(i)
	}

	// 绛夊緟鎵€鏈?goroutine 瀹屾垚
	for i := 0; i < 5; i++ {
		<-done
	}

	// 楠岃瘉鎵€鏈?Agent 閮藉凡娉ㄥ唽
	expectedCount := 7 // 2涓垵濮?+ 5涓苟鍙?	if gateway.registry.Count() != expectedCount {
		t.Errorf("鏈熸湜 %d 涓?Agent锛屼絾鏈?%d 涓?, expectedCount, gateway.registry.Count())
	}
}
