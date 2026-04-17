package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultGatewayConfig(t *testing.T) {
	cfg := DefaultGatewayConfig()

	if cfg.Version != "1.0" {
		t.Errorf("Expected version '1.0', got %s", cfg.Version)
	}

	if cfg.DefaultAgent != "main" {
		t.Errorf("Expected default agent 'main', got %s", cfg.DefaultAgent)
	}

	if len(cfg.Routes) != 1 {
		t.Errorf("Expected 1 default route, got %d", len(cfg.Routes))
	}

	if cfg.Routes[0].Channel != "cli" || cfg.Routes[0].Agent != "main" {
		t.Errorf("Expected route for cli->main, got %s->%s", cfg.Routes[0].Channel, cfg.Routes[0].Agent)
	}

	if cfg.Retry.MaxRetries != 3 {
		t.Errorf("Expected MaxRetries=3, got %d", cfg.Retry.MaxRetries)
	}

	if cfg.Retry.RetryDelay != 5 {
		t.Errorf("Expected RetryDelay=5, got %d", cfg.Retry.RetryDelay)
	}

	if cfg.HealthCheck.Enabled != true {
		t.Errorf("Expected HealthCheck.Enabled=true, got %v", cfg.HealthCheck.Enabled)
	}
}

func TestLoadFromFileWithGateway(t *testing.T) {
	// 创建临时测试配置文件
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "test_config.json")

	configContent := `{
		"bus-buf-size": 500,
		"sub-buf-size": 100,
		"channels": {
			"cli": {
				"enabled": true,
				"allowed_ids": ["user"]
			}
		},
		"llm": {
			"api_key": "test-key",
			"base_url": "https://api.example.com",
			"model": "test-model"
		},
		"tools": {
			"mcp": {"enabled": false, "servers": {}},
			"web_search": {"enabled": false, "api_key": ""},
			"url_parse": {"enabled": false}
		},
		"gateway": {
			"version": "2.0",
			"default_agent": "test-agent",
			"routes": [
				{
					"channel": "test-channel",
					"agent": "test-agent",
					"conditions": {"key": "value"},
					"priority": 20
				}
			],
			"retry": {
				"max_retries": 5,
				"retry_delay": 10,
				"timeout": 60
			},
			"health_check": {
				"enabled": false,
				"interval": 60,
				"timeout": 20
			}
		}
	}`

	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write test config file: %v", err)
	}

	// 加载配置
	cfg, err := LoadFromFile(configPath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// 验证 Gateway 配置
	if cfg.Gateway.Version != "2.0" {
		t.Errorf("Expected gateway version '2.0', got %s", cfg.Gateway.Version)
	}

	if cfg.Gateway.DefaultAgent != "test-agent" {
		t.Errorf("Expected gateway default agent 'test-agent', got %s", cfg.Gateway.DefaultAgent)
	}

	if len(cfg.Gateway.Routes) != 1 {
		t.Errorf("Expected 1 gateway route, got %d", len(cfg.Gateway.Routes))
	}

	if cfg.Gateway.Routes[0].Channel != "test-channel" {
		t.Errorf("Expected route channel 'test-channel', got %s", cfg.Gateway.Routes[0].Channel)
	}

	if cfg.Gateway.Retry.MaxRetries != 5 {
		t.Errorf("Expected MaxRetries=5, got %d", cfg.Gateway.Retry.MaxRetries)
	}
}

func TestLoadFromFileWithoutGateway(t *testing.T) {
	// 创建临时测试配置文件（不包含 gateway 配置）
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "test_config_no_gateway.json")

	configContent := `{
		"bus-buf-size": 500,
		"sub-buf-size": 100,
		"channels": {
			"cli": {
				"enabled": true,
				"allowed_ids": ["user"]
			}
		},
		"llm": {
			"api_key": "test-key",
			"base_url": "https://api.example.com",
			"model": "test-model"
		},
		"tools": {
			"mcp": {"enabled": false, "servers": {}},
			"web_search": {"enabled": false, "api_key": ""},
			"url_parse": {"enabled": false}
		}
	}`

	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write test config file: %v", err)
	}

	// 加载配置
	cfg, err := LoadFromFile(configPath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// 验证 Gateway 配置有默认值
	if cfg.Gateway.Version != "1.0" {
		t.Errorf("Expected default gateway version '1.0', got %s", cfg.Gateway.Version)
	}

	if cfg.Gateway.DefaultAgent != "main" {
		t.Errorf("Expected default gateway agent 'main', got %s", cfg.Gateway.DefaultAgent)
	}
}

func TestConfigGetGatewayConfig(t *testing.T) {
	cfg := &Config{}

	// 测试空 Gateway 配置
	gwConfig := cfg.GetGatewayConfig()
	if gwConfig.Version != "1.0" {
		t.Errorf("Expected default version '1.0', got %s", gwConfig.Version)
	}

	// 测试有 Gateway 配置
	cfg.Gateway = GatewayConfig{
		Version:      "2.0",
		DefaultAgent: "custom-agent",
	}

	gwConfig2 := cfg.GetGatewayConfig()
	if gwConfig2.Version != "2.0" {
		t.Errorf("Expected version '2.0', got %s", gwConfig2.Version)
	}
}

func TestConfigGetRouteForChannel(t *testing.T) {
	cfg := &Config{
		Gateway: GatewayConfig{
			Version:      "1.0",
			DefaultAgent: "main",
			Routes: []GatewayRouteRule{
				{Channel: "cli", Agent: "main", Priority: 10},
				{Channel: "cli", Agent: "backup", Priority: 5},
				{Channel: "web", Agent: "web-agent", Priority: 15},
			},
		},
	}

	// 测试找到路由
	route, found := cfg.GetRouteForChannel("cli")
	if !found {
		t.Error("Expected to find route for channel 'cli'")
	}
	if route.Agent != "main" {
		t.Errorf("Expected agent 'main', got %s", route.Agent)
	}

	// 测试找到最高优先级的路由
	route2, found2 := cfg.GetRouteForChannel("web")
	if !found2 {
		t.Error("Expected to find route for channel 'web'")
	}
	if route2.Agent != "web-agent" {
		t.Errorf("Expected agent 'web-agent', got %s", route2.Agent)
	}

	// 测试找不到路由
	_, found3 := cfg.GetRouteForChannel("non-existent")
	if found3 {
		t.Error("Expected not to find route for 'non-existent' channel")
	}
}
