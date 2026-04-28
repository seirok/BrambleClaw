package structs

// MCPConfig MCP 工具配置
type MCPConfig struct {
	Enabled bool                       `json:"enabled"`
	Servers map[string]MCPServerConfig `json:"servers"`
}

// MCPServerConfig MCP 服务器配置
type MCPServerConfig struct {
	Enabled bool              `json:"enabled"`
	Type    string            `json:"type"` // "stdio" or "sse"
	Command string            `json:"command,omitempty"`
	Args    []string          `json:"args,omitempty"`
	EnvFile string            `json:"env_file,omitempty"`
	URL     string            `json:"url,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
}

// WebSearchConfig 网络搜索工具配置
type WebSearchConfig struct {
	Enabled bool   `json:"enabled"`
	APIKey  string `json:"api_key"`
}

// UrlParseConfig URL 解析工具配置
type UrlParseConfig struct {
	Enabled bool `json:"enabled"`
}

// ToolsConfig 工具配置
type ToolsConfig struct {
	MCP       MCPConfig       `json:"mcp"`
	WebSearch WebSearchConfig `json:"web_search"`
	UrlParse  UrlParseConfig  `json:"url_parse"`
	Sandbox   SandboxConfig   `json:"sandbox"`
}

// DefaultToolsConfig 返回默认工具配置
func DefaultToolsConfig() ToolsConfig {
	return ToolsConfig{
		MCP: MCPConfig{
			Enabled: false,
			Servers: make(map[string]MCPServerConfig),
		},
		WebSearch: WebSearchConfig{
			Enabled: false,
			APIKey:  "",
		},
		UrlParse: UrlParseConfig{
			Enabled: false,
		},
		Sandbox: DefaultSandboxConfig(),
	}
}
