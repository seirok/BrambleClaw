package sandbox

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// SandboxConfig 沙箱配置
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

// DefaultConfig 返回默认配置
func DefaultConfig() *SandboxConfig {
	return &SandboxConfig{
		Enabled:          true,
		Workspace:        "./workspace",
		AllowReadOutside: false,
		FileSystem: FileSystemConfig{
			AllowWritePaths: []string{
				`^/tmp/`,
				`^/var/tmp/`,
			},
			MaxFileSize:  100 * 1024 * 1024,      // 100MB
			MaxTotalSize: 1 * 1024 * 1024 * 1024, // 1GB
		},
		Execution: ExecutionConfig{
			AllowedCommands: []string{
				"ls", "cat", "grep", "head", "tail",
				"python3", "python", "node", "npm",
				"go", "git", "docker",
			},
			Timeout:       30 * time.Second,
			MaxOutputSize: 1024 * 1024, // 1MB
		},
		Audit: AuditConfig{
			Enabled:    true,
			LogPath:    "./logs/sandbox_audit.log",
			MaxSize:    100, // 100MB
			MaxBackups: 10,
		},
	}
}

// LoadConfig 从文件加载配置
func LoadConfig(path string) (*SandboxConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取配置文件失败(%s): %w", path, err)
	}

	config := DefaultConfig()
	if err := yaml.Unmarshal(data, config); err != nil {
		return nil, fmt.Errorf("解析配置文件失败(%s): %w", path, err)
	}

	// 验证配置
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("配置验证失败: %w", err)
	}

	return config, nil
}

// Validate 验证配置
func (c *SandboxConfig) Validate() error {
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
func (c *SandboxConfig) IsPathAllowed(path string) bool {
	// TODO: 实现路径匹配逻辑
	return true
}

// IsCommandAllowed 检查命令是否在白名单中
func (c *SandboxConfig) IsCommandAllowed(command string) bool {
	for _, allowed := range c.Execution.AllowedCommands {
		if allowed == command {
			return true
		}
	}
	return false
}
