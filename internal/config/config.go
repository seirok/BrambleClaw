package config

import "brambleclaw/internal/config/structs"

// 类型别名，保持向后兼容性
type LogConfig = structs.LogConfig
type LLMConfig = structs.LLMConfig
type AgentConfig = structs.AgentConfig
type SessionConfig = structs.SessionConfig
type ChannelConfig = structs.ChannelConfig
type ToolsConfig = structs.ToolsConfig
type GatewayConfig = structs.GatewayConfig
type CompactConfig = structs.CompactConfig
type SandboxConfig = structs.SandboxConfig
type MCPConfig = structs.MCPConfig
type MCPServerConfig = structs.MCPServerConfig
type WebSearchConfig = structs.WebSearchConfig
type UrlParseConfig = structs.UrlParseConfig
type AuditConfig = structs.AuditConfig
type FileSystemConfig = structs.FileSystemConfig
type ExecutionConfig = structs.ExecutionConfig
type GatewayRouteRule = structs.GatewayRouteRule
type GatewayRetryPolicy = structs.GatewayRetryPolicy
type GatewayHealthCheck = structs.GatewayHealthCheck

type Config struct {
	Log        structs.LogConfig       `json:"log"`
	BusBufSize int                     `json:"bus-buf-size"`
	SubBufSize int                     `json:"sub-buf-size"`
	Channels   structs.ChannelConfig   `json:"channels"`
	LLMConfig  structs.LLMConfig       `json:"llm"`
	Tools      structs.ToolsConfig     `json:"tools"`
	Gateway    structs.GatewayConfig   `json:"gateway"`
	Agents     []structs.AgentConfig   `json:"agents"`
	Session    structs.SessionConfig   `json:"session"`
	Workspace  string                  `json:"workspace"`
	Compact    structs.CompactConfig   `json:"compact"`
	Sandbox    structs.SandboxConfig   `json:"sandbox"`
}

func Load(path string) (*Config, error) {
	var loader *Loader
	if path != "" {
		loader = NewLoaderWithPath(path)
	} else {
		loader = NewLoader()
	}
	cfg, _, err := loader.Load()
	if err != nil {
		return nil, err
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// GetGatewayConfig 返回 Config 中的 Gateway 配置
// 如果 Gateway 配置为空，返回默认配置
func (c *Config) GetGatewayConfig() GatewayConfig {
	if c.Gateway.Version == "" {
		return structs.DefaultGatewayConfig()
	}
	return c.Gateway
}

// CheckConfig 检查配置的合法性（保持向后兼容性）
func (c *Config) CheckConfig() error {
	return c.Validate()
}

// 导出默认配置函数，保持向后兼容性
func DefaultGatewayConfig() GatewayConfig {
	return structs.DefaultGatewayConfig()
}

func DefaultSessionConfig() SessionConfig {
	return structs.DefaultSessionConfig()
}
