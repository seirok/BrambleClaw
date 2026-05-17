package config

import (
	util "neoclaw/internal"
	"neoclaw/internal/config/structs"
	"neoclaw/internal/logger"
	"os"
	"sync"
)

// 类型别名，保持向后兼容性
type LogConfig = structs.LogConfig
type LLMConfig = structs.LLMConfig
type AgentConfig = structs.AgentConfig
type SessionConfig = structs.SessionConfig
type ChannelConfig = structs.ChannelConfig
type ToolsConfig = structs.ToolsConfig
type GatewayConfig = structs.GatewayConfig
type CompactConfig = structs.CompactConfig
type SandboxConfig = structs.SandboxConfig
type MCPConfig = structs.MCPConfig
type MCPServerConfig = structs.MCPServerConfig
type WebSearchConfig = structs.WebSearchConfig
type UrlParseConfig = structs.UrlParseConfig
type AuditConfig = structs.AuditConfig
type FileSystemConfig = structs.FileSystemConfig
type ExecutionConfig = structs.ExecutionConfig
type GatewayRouteRule = structs.GatewayRouteRule
type GatewayRetryPolicy = structs.GatewayRetryPolicy
type GatewayHealthCheck = structs.GatewayHealthCheck
type HookConfig = structs.HookConfig
type HookDefaults = structs.HookDefaults
type HookDefinition = structs.HookDefinition
type ExternalConfig = structs.ExternalConfig
type HookType = structs.HookType
type SidebarConfig = structs.SidebarConfig
type SkillConfig = structs.SkillConfig
type CronConfig = structs.CronConfig

type Config struct {
	Log        structs.LogConfig     `json:"log" mapstructure:"log"`
	BusBufSize int                   `json:"bus-buf-size" mapstructure:"bus-buf-size"`
	SubBufSize int                   `json:"sub-buf-size" mapstructure:"sub-buf-size"`
	Channels   structs.ChannelConfig `json:"channels" mapstructure:"channels"`
	LLMConfig  structs.LLMConfig
	Tools      structs.ToolsConfig   `json:"tools" mapstructure:"tools"`
	Gateway    structs.GatewayConfig `json:"gateway" mapstructure:"gateway"`
	Agents     []structs.AgentConfig `json:"agents" mapstructure:"agents"`
	Session    structs.SessionConfig `json:"session" mapstructure:"session"`
	Compact    structs.CompactConfig `json:"compact" mapstructure:"compact"`
	Sandbox    structs.SandboxConfig `json:"sandbox" mapstructure:"sandbox"`
	Hooks      structs.HookConfig    `json:"hooks"`
	Sidebar    structs.SidebarConfig `json:"sidebar" mapstructure:"sidebar"`
	Skill      structs.SkillConfig   `json:"skill" mapstructure:"skill"`
}

var (
	// globalConfig 全局配置单例实例
	globalConfig *Config

	// once 确保单例只被初始化一次
	once sync.Once
)

func Get() *Config {
	once.Do(func() {
		if globalConfig == nil {
			initInternal()
		}
	})
	return globalConfig
}

// Init 显式初始化，通常在 main.go 中调用
func Init() {
	once.Do(func() {
		initInternal()
	})
}

func initInternal() {
	// 1. 环境变量检查 (Early Exit)
	if ok := util.CheckEnviromentVirable(); !ok {
		os.Exit(1)
	}
	// 2. 加载或创建配置文件
	configPath := util.GetGlobalConfigPath()
	// 3. 文件存在性检查
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		logger.L().Warn().Str("path", configPath).Msg("Config file missing, creating default")
		globalConfig = createDefaultConfig()
		// 持久化默认配置文件
		savePath := util.GetGlobalConfigPath()
		err := util.SaveStructToJSON(savePath, globalConfig)
		if err != nil {
			logger.L().Error().Err(err).Msg("Failed to save default config")
			return
		}
	} else {
		// 4. 加载并校验
		globalConfig = &Config{}
		loadAndValidateConfig()
	}
}

func loadAndValidateConfig() {
	// 加载配置文件
	if err := util.LoadJSONToStruct(util.GetGlobalConfigPath(), globalConfig); err != nil {
		logger.L().Error().Err(err).Msg("Failed to load global config")
	}

	ValidateGlobalConfig()
}

// ValidateGlobalConfig validates the global config and fills defaults.
func ValidateGlobalConfig() {
	if globalConfig == nil {
		return
	}

	hasError := false

	// Validate Log config
	globalConfig.Log.Validate()

	// Validate BusBufSize and SubBufSize
	if globalConfig.BusBufSize <= 0 {
		logger.L().Warn().Int("invalid_bus_buf_size", globalConfig.BusBufSize).Msg("Invalid bus_buf_size, using 100")
		globalConfig.BusBufSize = 100
	}
	if globalConfig.SubBufSize <= 0 {
		logger.L().Warn().Int("invalid_sub_buf_size", globalConfig.SubBufSize).Msg("Invalid sub_buf_size, using 20")
		globalConfig.SubBufSize = 20
	}

	// Validate Channels config
	globalConfig.Channels.Validate()

	// Validate LLMConfig
	if globalConfig.LLMConfig.Validate() {
		hasError = true
	}

	// Validate Tools config
	globalConfig.Tools.Validate()

	// Validate Gateway config
	if globalConfig.Gateway.Validate() {
		hasError = true
	}

	// Validate Agents
	if len(globalConfig.Agents) == 0 {
		logger.L().Error().Msg("Agents list is empty")
		hasError = true
		// Restore default agent
		defaultCfg := createDefaultConfig()
		globalConfig.Agents = defaultCfg.Agents
	} else {
		// Validate each agent
		agentNames := make(map[string]bool)
		for i := range globalConfig.Agents {
			if globalConfig.Agents[i].Validate() {
				hasError = true
			}
			name := globalConfig.Agents[i].Name
			if agentNames[name] {
				logger.L().Warn().Str("duplicate_agent", name).Msg("Duplicate agent name found")
			}
			agentNames[name] = true
		}

		// Verify default agent exists
		defaultAgent := globalConfig.Gateway.DefaultAgent
		if !agentNames[defaultAgent] {
			logger.L().Error().Str("missing_agent", defaultAgent).Msg("Default agent not found in agents list")
			hasError = true
		}
	}

	// Validate Session config
	globalConfig.Session.Validate()

	// Validate Compact config
	globalConfig.Compact.Validate()

	// Validate Sandbox config
	globalConfig.Sandbox.Validate()

	// Validate Hooks config
	globalConfig.Hooks.Validate()

	// Validate Sidebar config
	globalConfig.Sidebar.Validate()

	// Validate Skill config
	globalConfig.Skill.Validate()

	if hasError {
		logger.L().Warn().Msg("Config validation completed with some errors, defaults applied where possible")
	} else {
		logger.L().Debug().Msg("Config validation completed successfully")
	}
}
