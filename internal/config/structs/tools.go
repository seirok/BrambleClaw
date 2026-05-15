package structs

// MCPConfig MCP 工具配置
type MCPConfig struct {
	Enabled bool                       `json:"enabled" mapstructure:"enabled"`
	Servers map[string]MCPServerConfig `json:"servers" mapstructure:"servers"`
}

// MCPServerConfig MCP 服务器配置
type MCPServerConfig struct {
	Disabled bool              `json:"disabled" mapstructure:"disabled"`
	Type     string            `json:"type" mapstructure:"type"` // "stdio" or "sse"
	Command  string            `json:"command,omitempty" mapstructure:"command,omitempty"`
	Args     []string          `json:"args,omitempty" mapstructure:"args,omitempty"`
	EnvFile  string            `json:"env_file,omitempty" mapstructure:"env_file,omitempty"`
	URL      string            `json:"url,omitempty" mapstructure:"url,omitempty"`
	Headers  map[string]string `json:"headers,omitempty" mapstructure:"headers,omitempty"`
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

// GlobConfig Glob 工具配置
type GlobConfig struct {
	Enabled bool `json:"enabled" mapstructure:"enabled"`
}

// GrepConfig Grep 工具配置
type GrepConfig struct {
	Enabled bool `json:"enabled" mapstructure:"enabled"`
}

// ReadConfig 读取工具配置
type ReadConfig struct {
	Enabled bool `json:"enabled" mapstructure:"enabled"`
}

// WriteConfig 写入工具配置
type WriteConfig struct {
	Enabled bool `json:"enabled" mapstructure:"enabled"`
}

// ToolsConfig 工具配置
type ToolsConfig struct {
	MCP       MCPConfig       `json:"mcp" mapstructure:"mcp"`
	WebSearch WebSearchConfig `json:"web_search" mapstructure:"web_search"`
	UrlParse  UrlParseConfig  `json:"url_parse" mapstructure:"url_parse"`
	Glob      GlobConfig      `json:"glob" mapstructure:"glob"`
	Grep      GrepConfig      `json:"grep" mapstructure:"grep"`
	Read      ReadConfig      `json:"read" mapstructure:"read"`
	Write     WriteConfig     `json:"write" mapstructure:"write"`
}

// DefaultToolsConfig 返回默认工具配置
func DefaultToolsConfig() ToolsConfig {
	return ToolsConfig{
		MCP: MCPConfig{
			Enabled: true,
			Servers: make(map[string]MCPServerConfig),
		},
		WebSearch: WebSearchConfig{
			Enabled: false,
			APIKey:  "",
		},
		UrlParse: UrlParseConfig{
			Enabled: true,
		},
		Glob: GlobConfig{
			Enabled: true,
		},
		Grep: GrepConfig{
			Enabled: true,
		},
		Read: ReadConfig{
			Enabled: true,
		},
		Write: WriteConfig{
			Enabled: true,
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

	if !c.Glob.Enabled {
		c.Glob.Enabled = defaults.Glob.Enabled
	}

	if !c.Grep.Enabled {
		c.Grep.Enabled = defaults.Grep.Enabled
	}

	if !c.Read.Enabled {
		c.Read.Enabled = defaults.Read.Enabled
	}

	if !c.Write.Enabled {
		c.Write.Enabled = defaults.Write.Enabled
	}

	return false
}
