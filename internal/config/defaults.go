package config

import (
	"brambleclaw/internal/config/structs"
	"os"
)

func getDefaultConfig() *Config {
	// 获取当前工作目录作为默认工作空间（绝对路径）
	workspace, err := getDefaultWorkspace()
	if err != nil {
		workspace = "C:\\workspace" // 备用默认路径
	}

	cfg := &Config{
		Log:        structs.DefaultLogConfig(),
		BusBufSize: 100,
		SubBufSize: 20,
		Channels: structs.ChannelConfig{
			CLI: structs.CLIChannelConfig{
				Enabled:    true,
				AllowedIDs: []string{"*"},
			},
		},
		LLMConfig:  structs.DefaultLLMConfig(),
		Tools:      structs.DefaultToolsConfig(),
		Gateway:    structs.DefaultGatewayConfig(),
		Agents:     []structs.AgentConfig{},
		Session:    structs.DefaultSessionConfig(),
		Workspace:  workspace,
		Compact:    structs.DefaultCompactConfig(),
		Sandbox:    structs.DefaultSandboxConfig(),
	}
	return cfg
}

func getDefaultWorkspace() (string, error) {
	// 尝试获取当前工作目录的绝对路径
	wd, err := getCurrentWorkingDirectory()
	if err != nil {
		return "", err
	}
	return wd, nil
}

func getCurrentWorkingDirectory() (string, error) {
	// 实现获取当前工作目录的函数
	// 这里可以根据平台使用不同的方法
	// 为了简单起见，我们使用 Go 的标准库
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return wd, nil
}

func ensureAllDefaults(cfg *Config) *Config {
	if cfg.Log.Path == "" {
		cfg.Log = structs.DefaultLogConfig()
	}

	if cfg.BusBufSize == 0 {
		cfg.BusBufSize = 100
	}

	if cfg.SubBufSize == 0 {
		cfg.SubBufSize = 20
	}

	if cfg.Workspace == "" {
		cfg.Workspace = "workspace"
	}

	if cfg.LLMConfig.APIKey == "" {
		cfg.LLMConfig = structs.DefaultLLMConfig()
	}

	if cfg.Session.StorageFormat == "" {
		cfg.Session = structs.DefaultSessionConfig()
	}

	if cfg.Tools.MCP.Enabled == false {
		cfg.Tools = structs.DefaultToolsConfig()
	}

	if cfg.Gateway.Version == "" {
		cfg.Gateway = structs.DefaultGatewayConfig()
	}

	if cfg.Sandbox.Workspace == "" {
		cfg.Sandbox = structs.DefaultSandboxConfig()
	}

	if cfg.Compact.MaxSummaryLength == 0 {
		cfg.Compact = structs.DefaultCompactConfig()
	}

	return cfg
}
