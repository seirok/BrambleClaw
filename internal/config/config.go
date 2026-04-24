package config

import (
	"brambleclaw/internal/logger"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

const (
	// 默认配置文件名
	DefaultConfigFileName = "config.json"
	// 默认配置子目录
	DefaultConfigDir = "config"
	// 环境变量名
	EnvConfigPath = "BRAMBLECLAW_CONFIG"
	// App名称，用于构建配置路径
	AppName = "brambleclaw"
)

// ChannelConfig 通道配置结构体
type ChannelConfig struct {
	CLI struct {
		Enabled    bool     `json:"enabled"`
		AllowedIDs []string `json:"allowed_ids"`
	} `json:"cli"`
	// 可以在这里添加其他通道类型的配置，如微信、Telegram等
	// Weixin struct {
	//     Enabled    bool     `json:"enabled"`
	//     AllowedIDs []string `json:"allowed_ids"`
	//     AppID      string   `json:"app_id"`
	//     AppSecret  string   `json:"app_secret"`
	// } `json:"weixin"`
	// Telegram struct {
	//     Enabled    bool     `json:"enabled"`
	//     AllowedIDs []string `json:"allowed_ids"`
	//     Token      string   `json:"token"`
	// } `json:"telegram"`
}

type Config struct {
	Log        LogConfig     `json:"log"`
	BusBufSize int           `json:"bus-buf-size"`
	SubBufSize int           `json:"sub-buf-size"`
	Channels   ChannelConfig `json:"channels"`
	LLMConfig  LLMConfig     `json:"llm"`
	Tools      ToolsConfig   `json:"tools"`
	Gateway    GatewayConfig `json:"gateway"`
	Agents     []AgentConfig `json:"agents"`    // Agent 配置列表
	Session    SessionConfig `json:"session"`   // Session 配置
	Workspace  string        `json:"workspace"` // 全局默认 workspace 根目录
	Compact    CompactConfig `json:"compact"`
	Sandbox    SandboxConfig `json:"sandbox"`
}

// Loader 配置加载器，支持多路径搜索
type Loader struct {
	// 显式指定的路径（最高优先级）
	ExplicitPath string
	// 搜索路径列表（按优先级排序）
	SearchPaths []string
}

// NewLoader 创建默认的配置加载器
func NewLoader() *Loader {
	return &Loader{
		SearchPaths: buildDefaultSearchPaths(),
	}
}

// NewLoaderWithPath 使用显式路径创建加载器
func NewLoaderWithPath(path string) *Loader {
	return &Loader{
		ExplicitPath: path,
		SearchPaths:  buildDefaultSearchPaths(),
	}
}

// buildDefaultSearchPaths 构建默认的搜索路径列表
func buildDefaultSearchPaths() []string {
	var paths []string

	// 1. 当前工作目录下的 config/config.json
	if cwd, err := os.Getwd(); err == nil {
		paths = append(paths, filepath.Join(cwd, DefaultConfigDir, DefaultConfigFileName))
		paths = append(paths, filepath.Join(cwd, DefaultConfigFileName))
	}

	// 2. 用户配置目录
	if userConfigDir, err := os.UserConfigDir(); err == nil {
		paths = append(paths, filepath.Join(userConfigDir, AppName, DefaultConfigFileName))
	}

	// 3. 用户主目录下的隐藏配置
	if homeDir, err := os.UserHomeDir(); err == nil {
		paths = append(paths, filepath.Join(homeDir, "."+AppName, DefaultConfigFileName))
		paths = append(paths, filepath.Join(homeDir, "."+AppName+".json"))
	}

	// 4. 可执行文件所在目录（适用于便携部署）
	if exePath, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exePath)
		paths = append(paths, filepath.Join(exeDir, DefaultConfigDir, DefaultConfigFileName))
		paths = append(paths, filepath.Join(exeDir, DefaultConfigFileName))
	}

	// 5. 运行时目录（开发/测试场景）
	_, filename, _, ok := runtime.Caller(0)
	if ok {
		runtimeDir := filepath.Dir(filename)
		paths = append(paths, filepath.Join(runtimeDir, "..", "..", DefaultConfigDir, DefaultConfigFileName))
	}

	return paths
}

func (c *Config) CheckConfig() error {
	// 1. 核心防御：必须是绝对路径
	// 这能直接干掉 "C:Users" 这种没有根斜杠的写法，也能干掉 "./data" 这种写法
	if !filepath.IsAbs(c.Workspace) {
		return fmt.Errorf("工作空间路径必须是绝对路径 (例如 C:\\workspace)，当前配置为: %s", c.Workspace)
	}

	// 2. 检查目录是否存在
	info, err := os.Stat(c.Workspace)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("工作空间目录不存在: %s", c.Workspace)
		}
		return err
	}

	// 3. 确保它不是个文件
	if !info.IsDir() {
		return fmt.Errorf("工作空间路径必须是一个目录，而非文件: %s", c.Workspace)
	}

	return nil
}

// Load 尝试从多个位置加载配置
// 优先级：ExplicitPath > 环境变量 > SearchPaths
func (l *Loader) Load() (*Config, string, error) {
	// 1. 检查显式指定的路径
	logger.L().Debug().Msg("显示加载")
	if l.ExplicitPath != "" {
		if cfg, err := LoadFromFile(l.ExplicitPath); err == nil {
			logger.L().Debug().Str("path", l.ExplicitPath).Msg("从显式路径加载配置成功")
			return cfg, l.ExplicitPath, nil
		} else {
			return nil, "", fmt.Errorf("显式指定的配置文件加载失败: %w", err)
		}
	}

	// 2. 检查环境变量
	logger.L().Debug().Msg("环境变量加载")
	if envPath := os.Getenv(EnvConfigPath); envPath != "" {
		if cfg, err := LoadFromFile(envPath); err == nil {
			logger.L().Debug().Str("path", envPath).Msg("从环境变量加载配置成功")
			return cfg, envPath, nil
		}
		// 环境变量存在但文件不存在，记录警告但不终止
		logger.L().Warn().Str("env", EnvConfigPath).Str("path", envPath).Msg("环境变量指定的配置文件不存在，尝试其他路径")
	}

	// 3. 按顺序搜索默认路径
	logger.L().Debug().Msg("默认加载")
	for _, path := range l.SearchPaths {
		if cfg, err := LoadFromFile(path); err == nil {
			logger.L().Debug().Str("path", path).Msg("从搜索路径加载配置成功")
			return cfg, path, nil
		}
	}

	// 4. 所有路径都失败了
	return nil, "", fmt.Errorf("无法找到配置文件，已尝试以下路径: %v", l.SearchPaths)
}

// LoadFromFile 从指定文件路径加载配置
// 底层职责：文件读取和 JSON 解析错误需要包装
func LoadFromFile(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取配置文件失败(%s): %w", path, err)
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("JSON解析失败(%s): %w", path, err)
	}

	// 设置 Gateway 默认值
	cfg = ensureGatewayDefaults(cfg)
	// 设置 Log 默认值
	cfg = ensureLogDefaults(cfg)
	// 设置 Workspace 默认值
	cfg = ensureWorkspaceDefaults(cfg)
	// 设置 Compact 默认值
	cfg = ensureCompactDefaults(cfg)

	return &cfg, nil
}

// ensureLogDefaults 确保日志配置有默认值
func ensureLogDefaults(cfg Config) Config {
	if cfg.Log.Path == "" {
		cfg.Log.Path = "logs/brambleclaw.log"
	}
	if cfg.Log.Level == "" {
		cfg.Log.Level = "debug"
	}
	// ConsoleEnabled 默认为 false，无需特殊处理
	return cfg
}

// ensureWorkspaceDefaults 确保 workspace 配置有默认值
func ensureWorkspaceDefaults(cfg Config) Config {
	if cfg.Workspace == "" {
		cfg.Workspace = "workspace"
	}
	return cfg
}

// ensureCompactDefaults 确保 Compact 配置具有合理的默认值
func ensureCompactDefaults(cfg Config) Config {
	// 设置 MaxSummaryLength 默认值
	if cfg.Compact.MaxSummaryLength == 0 {
		cfg.Compact.MaxSummaryLength = 10000
	}
	// 设置 HierarchicalDepth 默认值
	if cfg.Compact.HierarchicalDepth == 0 {
		cfg.Compact.HierarchicalDepth = 3
	}
	// 设置 CompactThreshold 默认值
	if cfg.Compact.CompactThreshold == 0 {
		cfg.Compact.CompactThreshold = 4000
	}
	// 设置 CompactRounds 默认值
	if cfg.Compact.CompactRounds == 0 {
		cfg.Compact.CompactRounds = 20
	}
	return cfg
}

// ensureGatewayDefaults 确保 Gateway 配置具有合理的默认值
func ensureGatewayDefaults(cfg Config) Config {
	// 如果 Gateway 配置为空（零值），设置默认值
	if cfg.Gateway.Version == "" {
		cfg.Gateway = DefaultGatewayConfig()
	}

	// 确保默认 Agent 有值
	if cfg.Gateway.DefaultAgent == "" {
		cfg.Gateway.DefaultAgent = "main"
	}

	// 设置重试策略默认值
	if cfg.Gateway.Retry.MaxRetries == 0 {
		cfg.Gateway.Retry.MaxRetries = 3
	}
	if cfg.Gateway.Retry.RetryDelay == 0 {
		cfg.Gateway.Retry.RetryDelay = 5
	}
	if cfg.Gateway.Retry.Timeout == 0 {
		cfg.Gateway.Retry.Timeout = 30
	}

	// 设置健康检查默认值
	if cfg.Gateway.HealthCheck.Interval == 0 {
		cfg.Gateway.HealthCheck.Interval = 30
	}
	if cfg.Gateway.HealthCheck.Timeout == 0 {
		cfg.Gateway.HealthCheck.Timeout = 10
	}

	return cfg
}

// Load 保留原有函数签名，使用默认加载器
// 注意：为了向后兼容，这个函数仍然接受path参数
func Load(path string) (*Config, error) {
	loader := NewLoader()
	if path != "" {
		loader.ExplicitPath = path
	}
	cfg, _, err := loader.Load()
	return cfg, err
}

type CompactConfig struct {
	CompactThreshold    int  `json:"compact_threshold"`     // Token threshold to trigger compaction
	CompactRounds       int  `json:"compact_rounds"`        // Message interval to trigger compaction
	MaxSummaryLength    int  `json:"max_summary_length"`    // Max chars per summary (default 10000)
	EnableHierarchical  bool `json:"enable_hierarchical"`   // Enable summary-of-summaries
	HierarchicalDepth   int  `json:"hierarchical_depth"`    // Max depth (default 3)
	ArchiveOldSummaries bool `json:"archive_old_summaries"` // Archive vs delete old summaries
	PreserveKeyContext  bool `json:"preserve_key_context"`  // Keep decisions/actions in compression
}

type LogConfig struct {
	Path           string `json:"path"`
	ConsoleEnabled bool   `json:"console_enabled"`
	Level          string `json:"level"`
}

type ToolsConfig struct {
	MCP       MCPConfig       `json:"mcp"`
	WebSearch WebSearchConfig `json:"web_search"`
	UrlParse  UrlParseConfig  `json:"url_parse"`
	Sandbox   SandboxConfig   `json:"sandbox"`
}

// SandboxConfig 沙箱工具配置

type UrlParseConfig struct {
	Enabled bool `json:"enabled"`
}

type WebSearchConfig struct {
	Enabled bool   `json:"enabled"`
	APIKey  string `json:"api_key"`
}

type MCPConfig struct {
	Enabled bool                       `json:"enabled"`
	Servers map[string]MCPServerConfig `json:"servers"`
}

type MCPServerConfig struct {
	Enabled bool              `json:"enabled"`
	Type    string            `json:"type"` // "stdio" or "sse"
	Command string            `json:"command,omitempty"`
	Args    []string          `json:"args,omitempty"`
	EnvFile string            `json:"env_file,omitempty"`
	URL     string            `json:"url,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
}

// GatewayRouteRule 路由规则配置
type GatewayRouteRule struct {
	Channel    string            `json:"channel"`    // 通道名称，如 weixin, telegram
	Agent      string            `json:"agent"`      // Agent 名称
	Conditions map[string]string `json:"conditions"` // 路由条件，如用户ID、关键词等
	Priority   int               `json:"priority"`   // 优先级，数值越大优先级越高
}

// GatewayRetryPolicy 重试策略配置
type GatewayRetryPolicy struct {
	MaxRetries int `json:"max_retries"` // 最大重试次数
	RetryDelay int `json:"retry_delay"` // 重试间隔（秒）
	Timeout    int `json:"timeout"`     // 超时时间（秒）
}

// GatewayHealthCheck 通道健康检查配置
type GatewayHealthCheck struct {
	Enabled  bool `json:"enabled"`  // 是否启用健康检查
	Interval int  `json:"interval"` // 检查间隔（秒）
	Timeout  int  `json:"timeout"`  // 检查超时（秒）
}

// GatewayConfig Gateway 完整配置
type GatewayConfig struct {
	Version      string             `json:"version"`       // 配置版本
	DefaultAgent string             `json:"default_agent"` // 默认 Agent
	Routes       []GatewayRouteRule `json:"routes"`        // 路由规则列表
	Retry        GatewayRetryPolicy `json:"retry"`         // 重试策略
	HealthCheck  GatewayHealthCheck `json:"health_check"`  // 健康检查配置
}

type SandboxConfig struct {
	// 基础配置
	Enabled          bool   `yaml:"enabled" json:"enabled"`                       // 是否启用沙箱
	Workspace        string `yaml:"workspace" json:"workspace"`                   // 工作目录路径
	AllowReadOutside bool   `yaml:"allow_read_outside" json:"allow_read_outside"` // 是否允许读取工作目录外的文件

	// 文件系统限制
	FileSystem FileSystemConfig `yaml:"filesystem" json:"filesystem"`

	// 执行限制
	Execution ExecutionConfig `yaml:"execution" json:"execution"`

	// 审计配置
	Audit AuditConfig `yaml:"audit" json:"audit"`
}

// FileSystemConfig 文件系统配置
type FileSystemConfig struct {
	AllowWritePaths []string `yaml:"allow_write_paths" json:"allow_write_paths"` // 允许写入的路径列表（正则表达式）
	MaxFileSize     int64    `yaml:"max_file_size" json:"max_file_size"`         // 最大文件大小（字节）
	MaxTotalSize    int64    `yaml:"max_total_size" json:"max_total_size"`       // 最大总文件大小（字节）
}

// ExecutionConfig 执行配置
type ExecutionConfig struct {
	AllowedCommands []string      `yaml:"allowed_commands" json:"allowed_commands"` // 允许执行的命令白名单
	Timeout         time.Duration `yaml:"timeout" json:"timeout"`                   // 命令执行超时时间
	MaxOutputSize   int           `yaml:"max_output_size" json:"max_output_size"`   // 最大输出大小（字节）
}

// AuditConfig 审计配置
type AuditConfig struct {
	Enabled    bool   `yaml:"enabled" json:"enabled"`         // 是否启用审计
	LogPath    string `yaml:"log_path" json:"log_path"`       // 审计日志路径
	MaxSize    int    `yaml:"max_size" json:"max_size"`       // 单个日志文件最大大小（MB）
	MaxBackups int    `yaml:"max_backups" json:"max_backups"` // 最大备份数量
}

// DefaultGatewayConfig 返回默认的 Gateway 配置
func DefaultGatewayConfig() GatewayConfig {
	return GatewayConfig{
		Version:      "1.0",
		DefaultAgent: "main",
		Routes: []GatewayRouteRule{
			{
				Channel:    "cli",
				Agent:      "main",
				Conditions: map[string]string{},
				Priority:   10,
			},
		},
		Retry: GatewayRetryPolicy{
			MaxRetries: 3,
			RetryDelay: 5,
			Timeout:    30,
		},
		HealthCheck: GatewayHealthCheck{
			Enabled:  true,
			Interval: 30,
			Timeout:  10,
		},
	}
}

// GetGatewayConfig 返回 Config 中的 Gateway 配置
// 如果 Gateway 配置为空，返回默认配置
func (c *Config) GetGatewayConfig() GatewayConfig {
	if c.Gateway.Version == "" {
		return DefaultGatewayConfig()
	}
	return c.Gateway
}
