package gateway

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// RouteRule 路由规则配置
type RouteRule struct {
	Channel    string            `yaml:"channel"`    // 通道名称，如 weixin, telegram
	Agent      string            `yaml:"agent"`      // Agent 名称
	Conditions map[string]string `yaml:"conditions"` // 路由条件，如用户ID、关键词等
	Priority   int               `yaml:"priority"`   // 优先级，数值越大优先级越高
}

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
