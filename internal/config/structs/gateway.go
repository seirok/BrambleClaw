package structs

// GatewayRouteRule 路由规则配置
type GatewayRouteRule struct {
	Channel    string            `json:"channel" mapstructure:"channel"`       // 通道名称，如 weixin, telegram
	Agent      string            `json:"agent" mapstructure:"agent"`           // Agent 名称
	Conditions map[string]string `json:"conditions" mapstructure:"conditions"` // 路由条件，如用户ID、关键词等
	Priority   int               `json:"priority" mapstructure:"priority"`     // 优先级，数值越大优先级越高
}

// GatewayRetryPolicy 重试策略配置
type GatewayRetryPolicy struct {
	MaxRetries int `json:"max_retries" mapstructure:"max_retries"` // 最大重试次数
	RetryDelay int `json:"retry_delay" mapstructure:"retry_delay"` // 重试间隔（秒）
	Timeout    int `json:"timeout" mapstructure:"timeout"`         // 超时时间（秒）
}

// GatewayHealthCheck 通道健康检查配置
type GatewayHealthCheck struct {
	Enabled  bool `json:"enabled" mapstructure:"enabled"`   // 是否启用健康检查
	Interval int  `json:"interval" mapstructure:"interval"` // 检查间隔（秒）
	Timeout  int  `json:"timeout" mapstructure:"timeout"`   // 检查超时（秒）
}

// GatewayConfig Gateway 完整配置
type GatewayConfig struct {
	Version      string             `json:"version" mapstructure:"version"`             // 配置版本
	DefaultAgent string             `json:"default_agent" mapstructure:"default_agent"` // 默认 Agent
	Routes       []GatewayRouteRule `json:"routes" mapstructure:"routes"`               // 路由规则列表
	Retry        GatewayRetryPolicy `json:"retry" mapstructure:"retry"`                 // 重试策略
	HealthCheck  GatewayHealthCheck `json:"health_check" mapstructure:"health_check"`   // 健康检查配置
}

// DefaultGatewayConfig 返回默认的 Gateway 配置
func DefaultGatewayConfig() GatewayConfig {
	return GatewayConfig{
		Version:      "1.0",
		DefaultAgent: "main",
		Routes: []GatewayRouteRule{
			{
				Channel:    "cli",
				Agent:      "main",
				Conditions: map[string]string{},
				Priority:   10,
			},
		},
		Retry: GatewayRetryPolicy{
			MaxRetries: 3,
			RetryDelay: 5,
			Timeout:    30,
		},
		HealthCheck: GatewayHealthCheck{
			Enabled:  true,
			Interval: 30,
			Timeout:  10,
		},
	}
}
