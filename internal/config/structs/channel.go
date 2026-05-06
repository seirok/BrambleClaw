package structs

// CLIChannelConfig CLI 通道配置
type CLIChannelConfig struct {
	Enabled    bool     `json:"enabled" mapstructure:"enabled"`
	AllowedIDs []string `json:"allowed_ids" mapstructure:"allowed_ids"`
}

// ChannelConfig 通道配置结构体
type ChannelConfig struct {
	CLI      CLIChannelConfig `json:"cli" mapstructure:"cli"`
	DingTalk DingTalkConfig   `json:"dingtalk" mapstructure:"dingtalk"`
}

type DingTalkConfig struct {
	Enabled            bool               `json:"enabled"                `
	ClientID           string             `json:"client_id"               `
	ClientSecret       string             `json:"client_secret,omitzero"  `
	AllowFrom          []string           `json:"allow_from"             `
	GroupTrigger       GroupTriggerConfig `json:"group_trigger,omitempty" `
	ReasoningChannelID string             `json:"reasoning_channel_id"    `
}

type GroupTriggerConfig struct {
	MentionOnly bool     `json:"mention_only,omitempty"`
	Prefixes    []string `json:"prefixes,omitempty"`
}
