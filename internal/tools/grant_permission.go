package tools

import (
	"brambleclaw/internal/logger"
	"brambleclaw/internal/sandbox"
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
)

// GrantPermissionTool 授予 session 级别的临时权限
type GrantPermissionTool struct {
	sandbox *sandbox.Sandbox
	name    string
	desc    string
}

// NewGrantPermissionTool 创建授权工具
func NewGrantPermissionTool(sandbox *sandbox.Sandbox) *GrantPermissionTool {
	return &GrantPermissionTool{
		sandbox: sandbox,
		name:    "grant_permission",
		desc:    "Grant temporary permissions (valid only for current session). Can grant write access to paths outside the working directory, or execute commands not in the whitelist. Only call this tool after explicit user consent.",
	}
}

// Name 返回工具名称
func (t *GrantPermissionTool) Name() string {
	return t.name
}

// Description 返回工具描述
func (t *GrantPermissionTool) Description() string {
	return t.desc
}

// Parameters 返回工具参数定义
func (t *GrantPermissionTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "Path to grant write access (must be a path returned by write tool that requires confirmation)",
			},
			"command": map[string]interface{}{
				"type":        "string",
				"description": "Command name to grant execution access (must be a command returned by shell tool that requires confirmation)",
			},
		},
		"required": []string{},
	}
}

// Execute 执行工具
func (t *GrantPermissionTool) Execute(ctx context.Context, argStr string) (interface{}, error) {
	logger.L().Info().Str("tool", t.name).Msg("tool start to execute")

	var args map[string]interface{}
	if err := json.Unmarshal([]byte(argStr), &args); err != nil {
		return nil, fmt.Errorf("Failed to parse parameters: %w", err)
	}

	sessionKey := sandbox.SessionKeyFromContext(ctx)
	if sessionKey == "" {
		return nil, fmt.Errorf("Failed to get session info, authorization failed")
	}

	path, hasPath := args["path"].(string)
	command, hasCommand := args["command"].(string)

	if !hasPath && !hasCommand {
		return nil, fmt.Errorf("Must provide path or command parameter")
	}

	var results []map[string]interface{}

	// 授权路径写入
	if hasPath && path != "" {
		absPath, err := filepath.Abs(path)
		if err != nil {
			return nil, fmt.Errorf("Failed to parse path: %w", err)
		}

		t.sandbox.Permissions().Grant(sessionKey, absPath)
		t.sandbox.LogAuditEvent(sandbox.AuditEventPermissionGrant, absPath, true,
			fmt.Sprintf("Session %s granted write access to %s", sessionKey, absPath))

		results = append(results, map[string]interface{}{
			"type":    "path",
			"status":  "granted",
			"path":    absPath,
			"message": fmt.Sprintf("Write access granted to %s, you can retry the write operation", absPath),
		})
	}

	// 授权命令执行
	if hasCommand && command != "" {
		t.sandbox.Permissions().GrantCommand(sessionKey, command)
		t.sandbox.LogAuditEvent(sandbox.AuditEventCommandPermissionGrant, command, true,
			fmt.Sprintf("Session %s granted permission to execute command %s", sessionKey, command))

		results = append(results, map[string]interface{}{
			"type":    "command",
			"status":  "granted",
			"command": command,
			"message": fmt.Sprintf("Permission granted to execute command '%s', you can retry execution", command),
		})
	}

	// 单个授权直接返回对象，多个授权返回数组
	if len(results) == 1 {
		return results[0], nil
	}
	return results, nil
}
