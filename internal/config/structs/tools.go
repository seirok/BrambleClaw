package structs

// MCPConfig MCP 工具配置
type MCPConfig struct {
	Enabled bool                       `json:"enabled" mapstructure:"enabled"`
	Servers map[string]MCPServerConfig `json:"servers" mapstructure:"servers"`
}

// MCPServerConfig MCP 服务器配置
type MCPServerConfig struct {
	Enabled bool              `json:"enabled" mapstructure:"enabled"`
	Type    string            `json:"type" mapstructure:"type"` // "stdio" or "sse"
	Command string            `json:"command,omitempty" mapstructure:"command,omitempty"`
	Args    []string          `json:"args,omitempty" mapstructure:"args,omitempty"`
	EnvFile string            `json:"env_file,omitempty" mapstructure:"env_file,omitempty"`
	URL     string            `json:"url,omitempty" mapstructure:"url,omitempty"`
	Headers map[string]string `json:"headers,omitempty" mapstructure:"headers,omitempty"`
}

// WebSearchConfig 网络搜索工具配置
type WebSearchConfig struct {
	Enabled bool   `json:"enabled" mapstructure:"enabled"`
	APIKey  string `json:"api_key" mapstructure:"api_key"`
}

// UrlParseConfig URL 解析工具配置
type UrlParseConfig struct {
	Enabled bool `json:"enabled" mapstructure:"enabled"`
}

// ToolsConfig 工具配置
type ToolsConfig struct {
	MCP       MCPConfig       `json:"mcp" mapstructure:"mcp"`
	WebSearch WebSearchConfig `json:"web_search" mapstructure:"web_search"`
	UrlParse  UrlParseConfig  `json:"url_parse" mapstructure:"url_parse"`
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
	}
}

// Validate validates ToolsConfig and fills defaults.
// Returns whether there was a critical error (never for ToolsConfig).
func (c *ToolsConfig) Validate() (hasError bool) {
	defaults := DefaultToolsConfig()

	if !c.MCP.Enabled {
		c.MCP.Enabled = defaults.MCP.Enabled
	}
	if c.MCP.Servers == nil {
		c.MCP.Servers = defaults.MCP.Servers
	}

	if !c.WebSearch.Enabled {
		c.WebSearch.Enabled = defaults.WebSearch.Enabled
	}

	if !c.UrlParse.Enabled {
		c.UrlParse.Enabled = defaults.UrlParse.Enabled
	}

	return false
}
