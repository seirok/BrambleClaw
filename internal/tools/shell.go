package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
)

// ShellTool Shell工具
type ShellTool struct {
	name        string
	description string
}

// NewShellTool 创建Shell工具
func NewShellTool() *ShellTool {
	return &ShellTool{
		name:        "shell",
		description: "Shell命令执行工具，用于执行Windows系统命令；",
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

func (t *ShellTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"command": map[string]interface{}{
				"type":        "string",
				"description": "要执行的Shell命令",
			},
		},
		"required": []string{"command"},
	}
}

// Execute 执行工具
func (t *ShellTool) Execute(ctx context.Context, argStr string) (interface{}, error) {
	// 获取命令
	var args map[string]interface{}
	err := json.Unmarshal([]byte(argStr), &args)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal arguments: %w", err)
	}
	cmdStr, ok := args["command"].(string)
	if !ok {
		return nil, errors.New("shell command is not string")
	}

	// 执行命令
	cmd := exec.CommandContext(ctx, "powershell", "-NoProfile", "-Command", cmdStr)
	output, err := cmd.CombinedOutput()
	outStr := string(output)
	result := map[string]interface{}{
		"output": string(output),
		"error":  err != nil,
	}

	if err != nil {
		result["error_message"] = outStr
		return result, fmt.Errorf("command [%s] failed: %w, detail: %s", cmdStr, err, outStr)
	}

	return result["output"], nil
}
