package config

import (
	util "brambleclaw/internal"
	"brambleclaw/internal/config/structs"
	"fmt"
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
		fmt.Printf("Config file missing at %s, creating default...", configPath)
		globalConfig = createDefaultConfig()
		// 持久化默认配置文件
		savePath := util.GetGlobalConfigPath()
		err := util.SaveStructToJSON(savePath, globalConfig)
		if err != nil {
			fmt.Println("Failed to save default config")
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
		fmt.Printf("Failed to load global config: %s", err.Error())
	}

	ValidateGlobalConfig()
}

func ValidateGlobalConfig() {
	// TODO: 校验配置
}

func DefaultHookConfig() HookConfig {
	return structs.DefaultHookConfig()
}
