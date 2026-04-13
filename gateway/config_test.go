package gateway

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	// 创建临时配置文件
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "gateway.yaml")

	configContent := `
version: "1.0"
default_agent: "main"
routes:
  - channel: "cli"
    agent: "main"
    priority: 10
  - channel: "weixin"
    agent: "customer_service"
    priority: 20
retry:
  max_retries: 3
  retry_delay: 5
  timeout: 30
health_check:
  enabled: true
  interval: 30
  timeout: 10
`
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("创建测试配置文件失败: %v", err)
	}

	// 测试加载配置
	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("加载配置失败: %v", err)
	}

	// 验证基本字段
	if cfg.Version != "1.0" {
		t.Errorf("Version 期望 '1.0', 得到 '%s'", cfg.Version)
	}
	if cfg.DefaultAgent != "main" {
		t.Errorf("DefaultAgent 期望 'main', 得到 '%s'", cfg.DefaultAgent)
	}

	// 验证路由规则
	if len(cfg.Routes) != 2 {
		t.Errorf("期望 2 条路由规则, 得到 %d", len(cfg.Routes))
	}

	// 验证重试策略
	if cfg.Retry.MaxRetries != 3 {
		t.Errorf("Retry.MaxRetries 期望 3, 得到 %d", cfg.Retry.MaxRetries)
	}

	// 验证健康检查
	if !cfg.HealthCheck.Enabled {
		t.Error("HealthCheck.Enabled 期望为 true")
	}
}

func TestLoadConfig_FileNotFound(t *testing.T) {
	_, err := LoadConfig("/nonexistent/path/config.yaml")
	if err == nil {
		t.Error("期望返回错误，但得到 nil")
	}
}

func TestLoadConfig_InvalidYAML(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "invalid.yaml")

	// 写入无效的 YAML
	if err := os.WriteFile(configPath, []byte("invalid: yaml: content: [}"), 0644); err != nil {
		t.Fatalf("创建测试文件失败: %v", err)
	}

	_, err := LoadConfig(configPath)
	if err == nil {
		t.Error("期望返回 YAML 解析错误，但得到 nil")
	}
}

func TestGatewayConfig_GetRouteForChannel(t *testing.T) {
	cfg := &GatewayConfig{
		Routes: []RouteRule{
			{Channel: "cli", Agent: "main", Priority: 10},
			{Channel: "weixin", Agent: "service", Priority: 20},
			{Channel: "cli", Agent: "backup", Priority: 5},
		},
	}

	// 测试匹配 cli 通道 - 应该返回优先级最高的 main
	route, found := cfg.GetRouteForChannel("cli")
	if !found {
		t.Error("应该找到 cli 通道的路由")
	}
	if route.Agent != "main" {
		t.Errorf("期望 Agent 为 'main'，得到 '%s'", route.Agent)
	}

	// 测试匹配 weixin 通道
	route, found = cfg.GetRouteForChannel("weixin")
	if !found {
		t.Error("应该找到 weixin 通道的路由")
	}
	if route.Agent != "service" {
		t.Errorf("期望 Agent 为 'service'，得到 '%s'", route.Agent)
	}

	// 测试未匹配的通道
	_, found = cfg.GetRouteForChannel("telegram")
	if found {
		t.Error("不应该找到未配置通道的路由")
	}
}
