package structs

// CLIChannelConfig CLI 通道配置
type CLIChannelConfig struct {
	Enabled    bool     `json:"enabled" mapstructure:"enabled"`
	AllowedIDs []string `json:"allowed_ids" mapstructure:"allowed_ids"`
}

// ChannelConfig 通道配置结构体
type DiscordConfig struct {
	Enabled            bool               `json:"enabled" mapstructure:"enabled"`
	BotToken           string             `json:"bot_token,omitzero" mapstructure:"bot_token" env:"PICOCLAW_CHANNELS_DISCORD_BOT_TOKEN"`
	AllowFrom          []string           `json:"allow_from" mapstructure:"allow_from"`
	GroupTrigger       GroupTriggerConfig `json:"group_trigger,omitempty" mapstructure:"group_trigger"`
	ReasoningChannelID string             `json:"reasoning_channel_id,omitzero" mapstructure:"reasoning_channel_id"`
}

type ChannelConfig struct {
	CLI      CLIChannelConfig `json:"cli" mapstructure:"cli"`
	DingTalk DingTalkConfig   `json:"dingtalk" mapstructure:"dingtalk"`
	Feishu   FeishuConfig     `json:"feishu" mapstructure:"feishu"`
	QQ       QQConfig         `json:"qq" mapstructure:"qq"`
	Discord  DiscordConfig    `json:"discord" mapstructure:"discord"`
	Telegram TelegramConfig   `json:"telegram" mapstructure:"telegram"`
	WeWork      WeWorkConfig      `json:"wework" mapstructure:"wework"`
	WeWorkWsBot WeWorkWsBotConfig `json:"wework_wsbot" mapstructure:"wework_wsbot"`
}

type TelegramConfig struct {
	Enabled             bool               `json:"enabled"                    env:"PICOCLAW_CHANNELS_TELEGRAM_ENABLED"`
	Token               string             `json:"token,omitzero"             env:"PICOCLAW_CHANNELS_TELEGRAM_TOKEN"`
	AllowFrom           []string           `json:"allow_from"                 env:"PICOCLAW_CHANNELS_TELEGRAM_ALLOW_FROM"`
	GroupTrigger        GroupTriggerConfig `json:"group_trigger,omitempty"`
	Proxy               string             `json:"proxy,omitzero"             env:"PICOCLAW_CHANNELS_TELEGRAM_PROXY"`
	BaseURL             string             `json:"base_url,omitzero"          env:"PICOCLAW_CHANNELS_TELEGRAM_BASE_URL"`
	ReasoningChannelID  string             `json:"reasoning_channel_id"       env:"PICOCLAW_CHANNELS_TELEGRAM_REASONING_CHANNEL_ID"`
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
	c.Discord.Validate()
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

// WeWorkConfig 企业微信 HTTP 回调通道配置
type WeWorkConfig struct {
	Enabled        bool     `json:"enabled" mapstructure:"enabled" env:"PICOCLAW_CHANNELS_WEWORK_ENABLED"`
	CorpID         string   `json:"corp_id" mapstructure:"corp_id" env:"PICOCLAW_CHANNELS_WEWORK_CORP_ID"`
	AgentID        string   `json:"agent_id" mapstructure:"agent_id" env:"PICOCLAW_CHANNELS_WEWORK_AGENT_ID"`
	Secret         string   `json:"secret,omitzero" mapstructure:"secret" env:"PICOCLAW_CHANNELS_WEWORK_SECRET"`
	Token          string   `json:"token,omitzero" mapstructure:"token" env:"PICOCLAW_CHANNELS_WEWORK_TOKEN"`
	EncodingAESKey string   `json:"encoding_aes_key,omitzero" mapstructure:"encoding_aes_key" env:"PICOCLAW_CHANNELS_WEWORK_ENCODING_AES_KEY"`
	WebhookPort    int      `json:"webhook_port" mapstructure:"webhook_port" env:"PICOCLAW_CHANNELS_WEWORK_WEBHOOK_PORT"`
	AllowFrom      []string `json:"allow_from" mapstructure:"allow_from" env:"PICOCLAW_CHANNELS_WEWORK_ALLOW_FROM"`
}

// WeWorkWsBotConfig 企业微信 WebSocket AI Bot 通道配置
type WeWorkWsBotConfig struct {
	Enabled        bool     `json:"enabled" mapstructure:"enabled" env:"PICOCLAW_CHANNELS_WEWORK_WSBOT_ENABLED"`
	BotID          string   `json:"bot_id" mapstructure:"bot_id" env:"PICOCLAW_CHANNELS_WEWORK_WSBOT_BOT_ID"`
	SecretID       string   `json:"secret_id,omitzero" mapstructure:"secret_id" env:"PICOCLAW_CHANNELS_WEWORK_WSBOT_SECRET_ID"`
	URL            string   `json:"url,omitzero" mapstructure:"url" env:"PICOCLAW_CHANNELS_WEWORK_WSBOT_URL"`
	Reconnect      bool     `json:"reconnect" mapstructure:"reconnect" env:"PICOCLAW_CHANNELS_WEWORK_WSBOT_RECONNECT"`
	ReconnectDelay int      `json:"reconnect_delay" mapstructure:"reconnect_delay" env:"PICOCLAW_CHANNELS_WEWORK_WSBOT_RECONNECT_DELAY"`
	Heartbeat      int      `json:"heartbeat" mapstructure:"heartbeat" env:"PICOCLAW_CHANNELS_WEWORK_WSBOT_HEARTBEAT"`
	AllowFrom      []string `json:"allow_from" mapstructure:"allow_from" env:"PICOCLAW_CHANNELS_WEWORK_WSBOT_ALLOW_FROM"`
}

// Validate validates DiscordConfig and fills defaults.
func (c *DiscordConfig) Validate() (hasError bool) {
	if c.AllowFrom == nil {
		c.AllowFrom = []string{}
	}
	c.GroupTrigger.Validate()
	return false
}
