package sandbox

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
)

// ShellTool 带沙箱的 Shell 命令执行工具
type ShellTool struct {
	sandbox     *Sandbox
	name        string
	description string
}

// NewShellTool 创建带沙箱的 Shell 工具
func NewShellTool(sandbox *Sandbox) *ShellTool {
	return &ShellTool{
		sandbox:     sandbox,
		name:        "shell",
		description: "带沙箱保护的 Shell 命令执行工具，只能执行白名单中的安全命令",
	}
}

// Name 返回工具名称
func (t *ShellTool) Name() string {
	return t.name
}

// Description 返回工具描述
func (t *ShellTool) Description() string {
	return t.description
}

// Parameters 返回工具参数定义
func (t *ShellTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"command": map[string]interface{}{
				"type":        "string",
				"description": "要执行的 Shell 命令（只能执行白名单中的命令）",
			},
			"working_dir": map[string]interface{}{
				"type":        "string",
				"description": "工作目录（可选，默认为沙箱工作目录）",
			},
		},
		"required": []string{"command"},
	}
}

// Execute 执行工具
func (t *ShellTool) Execute(ctx context.Context, argStr string) (interface{}, error) {
	// 解析参数
	var args struct {
		Command    string `json:"command"`
		WorkingDir string `json:"working_dir"`
	}

	if err := json.Unmarshal([]byte(argStr), &args); err != nil {
		return nil, fmt.Errorf("解析参数失败: %w", err)
	}

	if args.Command == "" {
		return nil, fmt.Errorf("命令不能为空")
	}

	// 在沙箱中执行命令
	output, err := t.sandbox.ExecuteCommand(ctx, args.Command)
	if err != nil {
		return map[string]interface{}{
			"output":        output,
			"error":         true,
			"error_message": err.Error(),
		}, fmt.Errorf("命令执行失败: %w", err)
	}

	return map[string]interface{}{
		"output": output,
		"error":  false,
	}, nil
}

// ValidateCommand 验证命令是否在白名单中（供外部使用）
func (t *ShellTool) ValidateCommand(command string) error {
	if t.sandbox == nil {
		return nil
	}
	return t.sandbox.ValidateCommand(command)
}

// extractCommandName 提取命令名称（与 sandbox.go 中的实现相同，保持兼容）
func extractCommandNameForShell(command string) string {
	// 去除首尾空白
	command = strings.TrimSpace(command)

	// 获取第一个单词（命令名称）
	parts := strings.Fields(command)
	if len(parts) == 0 {
		return ""
	}

	// 去除可能的引号
	cmd := strings.Trim(parts[0], `"'`)

	// 如果包含路径，只返回文件名
	return filepath.Base(cmd)
}
