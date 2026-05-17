package tools

import (
	"encoding/json"
	"errors"
	"fmt"
	"neoclaw/internal/interfaces"
	"neoclaw/internal/logger"
	"neoclaw/internal/registry"
	"neoclaw/internal/sandbox"
	"path/filepath"
)

// ToolRegistry 工具注册中心
var (
	ErrToolNotFound = errors.New("tool not found")
	ErrToolExists   = errors.New("tool already exists")
)

// 这一行强制要求编译器检查 ToolRegistry 是否实现了 Registry[Tool]
var _ interfaces.Registry[interfaces.Tool] = (*ToolRegistry)(nil)

type ToolRegistry struct {
	*registry.GenericRegistry[interfaces.Tool]
}

func NewToolRegistry() *ToolRegistry {
	return &ToolRegistry{
		GenericRegistry: registry.NewGenericRegistry[interfaces.Tool](
			func(name string) error { return fmt.Errorf("%w: %s", ErrToolExists, name) },
			func(name string) error { return fmt.Errorf("%w: %s", ErrToolNotFound, name) },
			nil,
		),
	}
}

// BaseTool 提供工具的通用基础方法
type BaseTool struct {
	name    string
	desc    string
	sandbox *sandbox.Sandbox
	params  map[string]interface{}
}

// NewBaseTool 创建基础工具
func NewBaseTool(name, desc string, sandbox *sandbox.Sandbox, params map[string]interface{}) *BaseTool {
	return &BaseTool{
		name:    name,
		desc:    desc,
		sandbox: sandbox,
		params:  params,
	}
}

func (b *BaseTool) Name() string      { return b.name }
func (b *BaseTool) Description() string { return b.desc }
func (b *BaseTool) Parameters() map[string]interface{} { return b.params }

// ResolvePath 解析路径（针对有沙箱的工具）
func (b *BaseTool) ResolvePath(path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	if b.sandbox != nil && b.sandbox.Config() != nil {
		return filepath.Join(b.sandbox.Config().Workspace, path)
	}
	return path
}

// ParseArgs 解析 JSON 参数
func (b *BaseTool) ParseArgs(argStr string) (map[string]interface{}, error) {
	var args map[string]interface{}
	if err := json.Unmarshal([]byte(argStr), &args); err != nil {
		return nil, fmt.Errorf("解析参数失败: %w", err)
	}
	return args, nil
}

// LogStart 记录工具开始执行
func (b *BaseTool) LogStart() {
	logger.L().Info().Str("tool", b.name).Msg("tool start to execute")
}

