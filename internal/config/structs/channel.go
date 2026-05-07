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
	Feishu   FeishuConfig     `json:"feishu" mapstructure:"feishu"`
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

type FeishuConfig struct {
	Enabled           bool               `json:"enabled"                     yaml:"-"                            env:"PICOCLAW_CHANNELS_FEISHU_ENABLED"`
	AppID             string             `json:"app_id"                      yaml:"-"                            env:"PICOCLAW_CHANNELS_FEISHU_APP_ID"`
	AppSecret         string             `json:"app_secret,omitzero"         yaml:"app_secret,omitempty"         env:"PICOCLAW_CHANNELS_FEISHU_APP_SECRET"`
	EncryptKey        string             `json:"encrypt_key,omitzero"        yaml:"encrypt_key,omitempty"        env:"PICOCLAW_CHANNELS_FEISHU_ENCRYPT_KEY"`
	VerificationToken string             `json:"verification_token,omitzero" yaml:"verification_token,omitempty" env:"PICOCLAW_CHANNELS_FEISHU_VERIFICATION_TOKEN"`
	AllowFrom         []string           `json:"allow_from"                  yaml:"-"                            env:"PICOCLAW_CHANNELS_FEISHU_ALLOW_FROM"`
	GroupTrigger      GroupTriggerConfig `json:"group_trigger,omitempty"     yaml:"-"`
	// Placeholder         PlaceholderConfig   `json:"placeholder,omitempty"       yaml:"-"`
	ReasoningChannelID string `json:"reasoning_channel_id"        yaml:"-"                            env:"PICOCLAW_CHANNELS_FEISHU_REASONING_CHANNEL_ID"`
	// RandomReactionEmoji FlexibleStringSlice `json:"random_reaction_emoji"       yaml:"-"                            env:"PICOCLAW_CHANNELS_FEISHU_RANDOM_REACTION_EMOJI"`
	IsLark bool `json:"is_lark"                     yaml:"-"                            env:"PICOCLAW_CHANNELS_FEISHU_IS_LARK"`
}
