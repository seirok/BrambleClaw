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
	QQ       QQConfig         `json:"qq" mapstructure:"qq"`
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

type QQConfig struct {
	Enabled              bool               `json:"enabled"                  yaml:"-"                    env:"PICOCLAW_CHANNELS_QQ_ENABLED"`
	AppID                string             `json:"app_id"                   yaml:"-"                    env:"PICOCLAW_CHANNELS_QQ_APP_ID"`
	AppSecret            string             `json:"app_secret,omitzero"      yaml:"app_secret,omitempty" env:"PICOCLAW_CHANNELS_QQ_APP_SECRET"`
	AllowFrom            []string           `json:"allow_from"               yaml:"-"                    env:"PICOCLAW_CHANNELS_QQ_ALLOW_FROM"`
	GroupTrigger         GroupTriggerConfig `json:"group_trigger,omitempty"  yaml:"-"`
	MaxMessageLength     int                `json:"max_message_length"       yaml:"-"                    env:"PICOCLAW_CHANNELS_QQ_MAX_MESSAGE_LENGTH"`
	MaxBase64FileSizeMiB int64              `json:"max_base64_file_size_mib" yaml:"-"                    env:"PICOCLAW_CHANNELS_QQ_MAX_BASE64_FILE_SIZE_MIB"`
	SendMarkdown         bool               `json:"send_markdown"            yaml:"-"                    env:"PICOCLAW_CHANNELS_QQ_SEND_MARKDOWN"`
	ReasoningChannelID   string             `json:"reasoning_channel_id"     yaml:"-"                    env:"PICOCLAW_CHANNELS_QQ_REASONING_CHANNEL_ID"`
}

// Validate validates ChannelConfig and fills defaults.
// Returns whether there was a critical error (never for ChannelConfig).
func (c *ChannelConfig) Validate() (hasError bool) {
	c.CLI.Validate()
	// Note: DingTalk, Feishu, QQ validation omitted for brevity,
	// but should follow the same pattern if needed
	return false
}

// Validate validates CLIChannelConfig and fills defaults.
func (c *CLIChannelConfig) Validate() (hasError bool) {
	if c.AllowedIDs == nil {
		c.AllowedIDs = []string{"*"}
	}
	return false
}

// Validate validates GroupTriggerConfig and fills defaults.
func (c *GroupTriggerConfig) Validate() (hasError bool) {
	if c.Prefixes == nil {
		c.Prefixes = []string{}
	}
	return false
}
