package sandbox

import (
	"brambleclaw/internal/config"
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// SandboxConfig 沙箱配置

// DefaultConfig 返回默认配置
func DefaultConfig() *config.SandboxConfig {
	return &config.SandboxConfig{
		Enabled:          true,
		Workspace:        "./workspace",
		AllowReadOutside: false,
		FileSystem: config.FileSystemConfig{
			AllowWritePaths: []string{
				`^/tmp/`,
				`^/var/tmp/`,
			},
			MaxFileSize:  100 * 1024 * 1024,      // 100MB
			MaxTotalSize: 1 * 1024 * 1024 * 1024, // 1GB
		},
		Execution: config.ExecutionConfig{
			AllowedCommands: []string{
				"ls", "cat", "grep", "head", "tail",
				"python3", "python", "node", "npm",
				"go", "git", "docker",
			},
			Timeout:       30 * time.Second,
			MaxOutputSize: 1024 * 1024, // 1MB
		},
		Audit: config.AuditConfig{
			Enabled:    true,
			LogPath:    "./logs/sandbox_audit.log",
			MaxSize:    100, // 100MB
			MaxBackups: 10,
		},
	}
}

// LoadConfig 从文件加载配置
func LoadConfig(path string) (*config.SandboxConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取配置文件失败(%s): %w", path, err)
	}

	cfg := DefaultConfig()
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("解析配置文件失败(%s): %w", path, err)
	}

	// 验证配置
	// if err := cfg.Validate(); err != nil {
	// 	return nil, fmt.Errorf("配置验证失败: %w", err)
	// }

	return cfg, nil
}

// Validate 验证配置
func ValidateConfig(c *config.SandboxConfig) error {
	if c.Workspace == "" {
		return fmt.Errorf("workspace 不能为空")
	}

	if c.Execution.Timeout <= 0 {
		return fmt.Errorf("execution.timeout 必须大于 0")
	}

	if c.FileSystem.MaxFileSize <= 0 {
		return fmt.Errorf("filesystem.max_file_size 必须大于 0")
	}

	return nil
}

// IsPathAllowed 检查路径是否在允许的写入列表中
func IsPathAllowed(c *config.SandboxConfig, path string) bool {
	// TODO: 实现路径匹配逻辑
	return true
}

// IsCommandAllowed 检查命令是否在白名单中
func IsCommandAllowed(c *config.SandboxConfig, command string) bool {
	for _, allowed := range c.Execution.AllowedCommands {
		if allowed == command {
			return true
		}
	}
	return false
}
