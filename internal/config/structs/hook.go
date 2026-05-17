package structs

import "neoclaw/internal/logger"

// HookType Hook 类型枚举
type HookType string

const (
	// HookTypeInternal 内部 Go 函数 Hook
	HookTypeInternal HookType = "internal"
	// HookTypeExternal 外部脚本 Hook
	HookTypeExternal HookType = "external"
)

// IsValid 检查 Hook 类型是否有效
func (t HookType) IsValid() bool {
	switch t {
	case HookTypeInternal, HookTypeExternal:
		return true
	}
	return false
}

// HookConfig Hook 配置根节点
type HookConfig struct {
	// Version 配置版本
	Version string `json:"version" yaml:"version"`

	// Defaults 全局默认设置
	Defaults HookDefaults `json:"defaults" yaml:"defaults"`

	// Definitions Hook 定义列表
	Definitions []HookDefinition `json:"definitions" yaml:"definitions"`

	// ThinkingVisibility 思考过程可视化配置
	ThinkingVisibility ThinkingVisibilityConfig `json:"thinking_visibility" yaml:"thinking_visibility"`
}

// HookDefaults 全局默认设置
type HookDefaults struct {
	// TimeoutMs 默认超时（毫秒）
	TimeoutMs int `json:"timeout_ms" yaml:"timeout_ms"`

	// MaxOutputSize 最大输出大小（字节），默认 1MB
	MaxOutputSize int `json:"max_output_size" yaml:"max_output_size"`

	// Shell 默认 shell
	Shell string `json:"shell" yaml:"shell"`

	// WorkingDir 默认工作目录
	WorkingDir string `json:"working_dir" yaml:"working_dir"`

	// Env 默认环境变量
	Env []string `json:"env" yaml:"env"`
}

// HookDefinition 单个 Hook 定义
type HookDefinition struct {
	// Point 钩子点名称，如 "order.before_save"
	Point string `json:"point" yaml:"point"`

	// Type Hook 类型: internal / external
	Type HookType `json:"type" yaml:"type"`

	// Enabled 是否启用
	Enabled bool `json:"enabled" yaml:"enabled"`

	// Priority 优先级，值越小优先级越高，默认 50
	Priority int `json:"priority" yaml:"priority"`

	// Config 外部脚本配置（仅当 Type=external 时有效）
	Config ExternalConfig `json:"config,omitempty" yaml:"config,omitempty"`
}

// ExternalConfig 外部脚本配置
type ExternalConfig struct {
	// Command 执行命令，如 "python3", "/bin/bash"
	Command string `json:"command" yaml:"command"`

	// ScriptPath 脚本路径
	ScriptPath string `json:"script_path" yaml:"script_path"`

	// Args 额外参数
	Args []string `json:"args" yaml:"args"`

	// TimeoutMs 超时（毫秒），0 表示使用默认值
	TimeoutMs int `json:"timeout_ms" yaml:"timeout_ms"`

	// WorkingDir 工作目录，空表示使用默认值
	WorkingDir string `json:"working_dir" yaml:"working_dir"`

	// Env 环境变量
	Env []string `json:"env" yaml:"env"`

	// MaxOutputSize 最大输出大小（字节），0 表示使用默认值
	MaxOutputSize int `json:"max_output_size" yaml:"max_output_size"`
}

// GetTimeoutMs 获取超时（毫秒）
// 如果未设置，使用默认值
func (c *ExternalConfig) GetTimeoutMs(defaultTimeout int) int {
	if c.TimeoutMs > 0 {
		return c.TimeoutMs
	}
	return defaultTimeout
}

// GetMaxOutputSize 获取最大输出大小（字节）
// 如果未设置，使用默认值
func (c *ExternalConfig) GetMaxOutputSize(defaultSize int) int {
	if c.MaxOutputSize > 0 {
		return c.MaxOutputSize
	}
	return defaultSize
}

// GetWorkingDir 获取工作目录
// 如果未设置，使用默认值
func (c *ExternalConfig) GetWorkingDir(defaultDir string) string {
	if c.WorkingDir != "" {
		return c.WorkingDir
	}
	return defaultDir
}

// ThinkingVisibilityConfig 思考过程可视化配置
type ThinkingVisibilityConfig struct {
	Enabled   bool                  `json:"enabled" yaml:"enabled"`
	MaxEvents int                   `json:"max_events" yaml:"max_events"`
	Points    []ThinkingPointConfig `json:"points" yaml:"points"`
}

// ThinkingPointConfig 单个 hook point 的可视化配置
type ThinkingPointConfig struct {
	Point     string `json:"point" yaml:"point"`
	Enabled   bool   `json:"enabled" yaml:"enabled"`
	Verbosity string `json:"verbosity" yaml:"verbosity"` // "summary" | "detail"
}

// DefaultHookConfig 返回默认 Hook 配置
func DefaultHookConfig() HookConfig {
	return HookConfig{
		Version: "1.0",
		Defaults: HookDefaults{
			TimeoutMs:     5000,
			MaxOutputSize: 1024 * 1024, // 1MB
			Shell:         "/bin/bash",
			WorkingDir:    "./scripts",
			Env:           []string{},
		},
		Definitions: []HookDefinition{},
		ThinkingVisibility: ThinkingVisibilityConfig{
			Enabled:   true,
			MaxEvents: 200,
			Points: []ThinkingPointConfig{
				{Point: "hook.point.llm.request", Enabled: true, Verbosity: "summary"},
				{Point: "hook.point.llm.response", Enabled: true, Verbosity: "summary"},
				{Point: "hook.point.llm.error", Enabled: true, Verbosity: "detail"},
				{Point: "hook.point.tool.pre-execute", Enabled: true, Verbosity: "detail"},
				{Point: "hook.point.tool.result", Enabled: true, Verbosity: "summary"},
				{Point: "hook.point.tool.error", Enabled: true, Verbosity: "detail"},
				{Point: "hook.point.message.pre-process", Enabled: false, Verbosity: "summary"},
				{Point: "hook.point.message.pre-response", Enabled: false, Verbosity: "summary"},
			},
		},
	}
}

// Validate validates HookConfig and fills defaults.
// Returns whether there was a critical error (never for HookConfig).
func (c *HookConfig) Validate() (hasError bool) {
	defaults := DefaultHookConfig()

	if c.Version == "" {
		c.Version = defaults.Version
	}

	if c.Definitions == nil {
		c.Definitions = defaults.Definitions
	}

	// Validate each hook definition
	for i := range c.Definitions {
		c.Definitions[i].Validate()
	}

	// Validate thinking visibility
	if !c.ThinkingVisibility.Enabled {
		c.ThinkingVisibility.Enabled = defaults.ThinkingVisibility.Enabled
	}
	if c.ThinkingVisibility.MaxEvents <= 0 {
		c.ThinkingVisibility.MaxEvents = defaults.ThinkingVisibility.MaxEvents
	}
	if c.ThinkingVisibility.Points == nil {
		c.ThinkingVisibility.Points = defaults.ThinkingVisibility.Points
	}

	return false
}

// Validate validates HookDefinition and fills defaults.
func (c *HookDefinition) Validate() (hasError bool) {
	if c.Point == "" {
		// Invalid hook, but not critical - will be skipped
		logger.L().Warn().Msg("Hook point is empty, skipping")
	}

	// Validate hook type
	if !c.Type.IsValid() {
		logger.L().Warn().Str("invalid_type", string(c.Type)).Msg("Invalid hook type, using 'internal'")
		c.Type = HookTypeInternal
	}

	// Validate priority
	if c.Priority <= 0 {
		c.Priority = 50
	} else if c.Priority > 100 {
		c.Priority = 100
	}

	// Validate external config if type is external
	if c.Type == HookTypeExternal {
		c.Config.Validate()
	}

	return false
}

// Validate validates ExternalConfig and fills defaults.
func (c *ExternalConfig) Validate() (hasError bool) {
	defaults := DefaultHookConfig().Defaults

	if c.Command == "" {
		c.Command = defaults.Shell
	}

	if c.ScriptPath == "" {
		logger.L().Warn().Msg("Hook script_path is empty")
	}

	// Timeout and MaxOutputSize use the Get* accessor methods already,
	// but we validate they're positive
	if c.TimeoutMs < 0 {
		c.TimeoutMs = 0 // 0 means use default
	}
	if c.MaxOutputSize < 0 {
		c.MaxOutputSize = 0 // 0 means use default
	}

	return false
}
