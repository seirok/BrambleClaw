package structs

import "time"

// FileSystemConfig 文件系统配置
type FileSystemConfig struct {
	AllowWritePaths []string `yaml:"allow_write_paths" json:"allow_write_paths" mapstructure:"allow_write_paths"` // 允许写入的路径列表（正则表达式）
	MaxFileSize     int64    `yaml:"max_file_size" json:"max_file_size" mapstructure:"max_file_size"`             // 最大文件大小（字节）
	MaxTotalSize    int64    `yaml:"max_total_size" json:"max_total_size" mapstructure:"max_total_size"`          // 最大总文件大小（字节）
}

// ExecutionConfig 执行配置
type ExecutionConfig struct {
	AllowedCommands []string      `yaml:"allowed_commands" json:"allowed_commands" mapstructure:"allowed_commands"` // 允许执行的命令白名单
	Timeout         time.Duration `yaml:"timeout" json:"timeout" mapstructure:"timeout"`                            // 命令执行超时时间
	MaxOutputSize   int           `yaml:"max_output_size" json:"max_output_size" mapstructure:"max_output_size"`    // 最大输出大小（字节）
}

// AuditConfig 审计配置
type AuditConfig struct {
	Enabled    bool   `yaml:"enabled" json:"enabled" mapstructure:"enabled"`             // 是否启用审计
	LogPath    string `yaml:"log_path" json:"log_path" mapstructure:"log_path"`          // 审计日志路径
	MaxSize    int    `yaml:"max_size" json:"max_size" mapstructure:"max_size"`          // 单个日志文件最大大小（MB）
	MaxBackups int    `yaml:"max_backups" json:"max_backups" mapstructure:"max_backups"` // 最大备份数量
}

// SandboxConfig 沙箱工具配置
type SandboxConfig struct {
	// 基础配置
	Enabled          bool   `yaml:"enabled" json:"enabled" mapstructure:"enabled"`                                  // 是否启用沙箱
	Workspace        string `yaml:"workspace" json:"workspace" mapstructure:"workspace"`                            // 工作目录路径
	AllowReadOutside bool   `yaml:"allow_read_outside" json:"allow_read_outside" mapstructure:"allow_read_outside"` // 是否允许读取工作目录外的文件

	// 文件系统限制
	FileSystem FileSystemConfig `yaml:"filesystem" json:"filesystem" mapstructure:"filesystem"`

	// 执行限制
	Execution ExecutionConfig `yaml:"execution" json:"execution" mapstructure:"execution"`

	// 审计配置
	Audit AuditConfig `yaml:"audit" json:"audit" mapstructure:"audit"`
}

// DefaultSandboxConfig 返回默认沙箱配置
func DefaultSandboxConfig() SandboxConfig {
	return SandboxConfig{
		Enabled:          false,
		Workspace:        "sandbox",
		AllowReadOutside: false,
		FileSystem: FileSystemConfig{
			AllowWritePaths: []string{},
			MaxFileSize:     10 * 1024 * 1024,  // 10MB
			MaxTotalSize:    100 * 1024 * 1024, // 100MB
		},
		Execution: ExecutionConfig{
			AllowedCommands: []string{},
			Timeout:         30 * time.Second,
			MaxOutputSize:   1 * 1024 * 1024, // 1MB
		},
		Audit: AuditConfig{
			Enabled:    false,
			LogPath:    "audit.log",
			MaxSize:    100,
			MaxBackups: 7,
		},
	}
}
