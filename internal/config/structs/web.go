package structs

// WebConfig Web HTTP server configuration
type WebConfig struct {
	Enabled        bool     `json:"enabled" mapstructure:"enabled"`                 // 是否启用 Web 服务
	Host           string   `json:"host" mapstructure:"host"`                       // 绑定地址，默认 "127.0.0.1"
	Port           int      `json:"port" mapstructure:"port"`                       // 监听端口，默认 8080
	AllowedOrigins []string `json:"allowed_origins" mapstructure:"allowed_origins"` // CORS 白名单
	APIKey         string   `json:"api_key" mapstructure:"api_key"`                 // 管理接口鉴权
}

// DefaultWebConfig returns the default Web configuration
func DefaultWebConfig() WebConfig {
	return WebConfig{
		Enabled:        true,
		Host:           "127.0.0.1",
		Port:           8080,
		AllowedOrigins: []string{},
		APIKey:         "",
	}
}

// Validate validates WebConfig and fills defaults
func (c *WebConfig) Validate() {
	defaults := DefaultWebConfig()
	if c.Host == "" {
		c.Host = defaults.Host
	}
	if c.Port <= 0 {
		c.Port = defaults.Port
	}
	if c.AllowedOrigins == nil {
		c.AllowedOrigins = defaults.AllowedOrigins
	}
}
