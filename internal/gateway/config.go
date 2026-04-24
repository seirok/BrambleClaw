package gateway

import (
	"brambleclaw/internal/config"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// RouteRule 路由规则配置

// RetryPolicy 重试策略配置
type RetryPolicy struct {
	MaxRetries int `yaml:"max_retries"` // 最大重试次数
	RetryDelay int `yaml:"retry_delay"` // 重试间隔（秒）
	Timeout    int `yaml:"timeout"`     // 超时时间（秒）
}

// ChannelHealthCheck 通道健康检查配置
type ChannelHealthCheck struct {
	Enabled  bool `yaml:"enabled"`  // 是否启用健康检查
	Interval int  `yaml:"interval"` // 检查间隔（秒）
	Timeout  int  `yaml:"timeout"`  // 检查超时（秒）
}

// GatewayConfig Gateway 完整配置
type GatewayConfig struct {
	Version      string             `yaml:"version"`       // 配置版本
	DefaultAgent string             `yaml:"default_agent"` // 默认 Agent
	Routes       []RouteRule        `yaml:"routes"`        // 路由规则列表
	Retry        RetryPolicy        `yaml:"retry"`         // 重试策略
	HealthCheck  ChannelHealthCheck `yaml:"health_check"`  // 健康检查配置
}

// LoadConfig 从 YAML 文件加载 Gateway 配置
// 底层职责：文件读取和解析错误需要包装
func LoadConfig(path string) (*GatewayConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取配置文件失败(%s): %w", path, err)
	}

	var cfg GatewayConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("解析 YAML 配置失败(%s): %w", path, err)
	}

	// 设置默认值
	if cfg.Retry.MaxRetries == 0 {
		cfg.Retry.MaxRetries = 3
	}
	if cfg.Retry.RetryDelay == 0 {
		cfg.Retry.RetryDelay = 5
	}
	if cfg.Retry.Timeout == 0 {
		cfg.Retry.Timeout = 30
	}
	if cfg.HealthCheck.Interval == 0 {
		cfg.HealthCheck.Interval = 30
	}
	if cfg.HealthCheck.Timeout == 0 {
		cfg.HealthCheck.Timeout = 10
	}

	return &cfg, nil
}

// LoadConfigFromConfig 从统一的 config.Config 加载 Gateway 配置
// 用于将主配置中的 Gateway 配置转换为 gateway.GatewayConfig
func LoadConfigFromConfig(cfg *config.Config) (*GatewayConfig, error) {
	gwConfig := cfg.GetGatewayConfig()

	// 转换 Routes
	routes := make([]RouteRule, len(gwConfig.Routes))
	for i, route := range gwConfig.Routes {
		routes[i] = RouteRule{
			Channel:    route.Channel,
			Agent:      route.Agent,
			Conditions: route.Conditions,
			Priority:   route.Priority,
		}
	}

	// 创建 GatewayConfig
	return &GatewayConfig{
		Version:      gwConfig.Version,
		DefaultAgent: gwConfig.DefaultAgent,
		Routes:       routes,
		Retry: RetryPolicy{
			MaxRetries: gwConfig.Retry.MaxRetries,
			RetryDelay: gwConfig.Retry.RetryDelay,
			Timeout:    gwConfig.Retry.Timeout,
		},
		HealthCheck: ChannelHealthCheck{
			Enabled:  gwConfig.HealthCheck.Enabled,
			Interval: gwConfig.HealthCheck.Interval,
			Timeout:  gwConfig.HealthCheck.Timeout,
		},
	}, nil
}

// GetRouteForChannel 获取指定通道的路由规则
// 返回匹配该通道且优先级最高的路由规则
func (c *GatewayConfig) GetRouteForChannel(channel string) (*RouteRule, bool) {
	var matched *RouteRule
	maxPriority := -1

	for i := range c.Routes {
		route := &c.Routes[i]
		if route.Channel == channel && route.Priority > maxPriority {
			matched = route
			maxPriority = route.Priority
		}
	}

	return matched, matched != nil
}
