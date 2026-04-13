package config

import (
	"encoding/json"
	"miniGoClaw/logger"
	"os"
)

type Config struct {
	BusBufSize int `json:"bus-buf-size"`
	SubBufSize int `json:"sub-buf-size"`
	Channels   struct {
		CLI struct {
			Enabled    bool     `json:"enabled"`
			AllowedIDs []string `json:"allowed_ids"`
		} `json:"cli"`
	} `json:"channels"`
	LLMConfig LLMConfig   `json:"llm"`
	Tools     ToolsConfig `json:"tools"`
}

type ToolsConfig struct {
	MCP       MCPConfig       `json:"mcp"`
	WebSearch WebSearchConfig `json:"web_search"`
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

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		logger.L().Error().Str("文件读取失败: %v", err.Error())
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		logger.L().Error().Str("JSON解析失败: %v", err.Error())
	}
	return &cfg, err
}
