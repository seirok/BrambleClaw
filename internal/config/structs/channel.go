package structs

// CLIChannelConfig CLI 通道配置
type CLIChannelConfig struct {
	Enabled    bool     `json:"enabled" mapstructure:"enabled"`
	AllowedIDs []string `json:"allowed_ids" mapstructure:"allowed_ids"`
}

// ChannelConfig 通道配置结构体
type ChannelConfig struct {
	CLI CLIChannelConfig `json:"cli" mapstructure:"cli"`
}
