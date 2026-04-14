package config

import (
	"encoding/json"
	"fmt"
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
	UrlParse  UrlParseConfig  `json:"url_parse"`
}

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

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		err = fmt.Errorf("文件读取失败(%s): %w", path, err)
		logger.L().Error().Err(err).Msg("")
		return nil, err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		err = fmt.Errorf("JSON解析失败(%s): %w", path, err)
		logger.L().Error().Err(err).Msg("")
		return nil, err
	}
	return &cfg, nil
}
