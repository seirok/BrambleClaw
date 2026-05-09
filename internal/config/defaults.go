package config

import (
	"brambleclaw/internal/config/structs"
)

func createDefaultConfig() *Config {
	cfg := &Config{}

	cfg.Log = structs.DefaultLogConfig()

	cfg.BusBufSize = 100

	cfg.SubBufSize = 20

	cfg.Channels = structs.ChannelConfig{
		CLI: structs.CLIChannelConfig{
			Enabled:    true,
			AllowedIDs: []string{"*"},
		},
	}

	cfg.Agents = []structs.AgentConfig{
		{
			Name:        "main",
			Description: "Default agent for handling messages",
			LLM:         structs.DefaultLLMConfig(),
			Tools:       []string{"web_search", "shell", "filesystem", "url_parse"},
			MaxHistory:  50,
			Enabled:     true,
		},
	}

	cfg.Session = structs.DefaultSessionConfig()

	cfg.Gateway = structs.DefaultGatewayConfig()

	cfg.Sandbox = structs.DefaultSandboxConfig()

	cfg.Compact = structs.DefaultCompactConfig()

	cfg.LLMConfig = structs.DefaultLLMConfig()

	cfg.Hooks = structs.DefaultHookConfig()

	return cfg
}
