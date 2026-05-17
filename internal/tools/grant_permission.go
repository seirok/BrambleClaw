package tools

import (
	"context"
	"fmt"
	"neoclaw/internal/sandbox"
	"path/filepath"
)

// GrantPermissionTool 授予 session 级别的临时权限
type GrantPermissionTool struct {
	*BaseTool
}

// NewGrantPermissionTool 创建授权工具
func NewGrantPermissionTool(sandbox *sandbox.Sandbox) *GrantPermissionTool {
	return &GrantPermissionTool{
		BaseTool: NewBaseTool(
			"grant_permission",
			"Grant temporary permissions (valid only for current session). Can grant write access to paths outside the working directory.",
			sandbox,
			map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]any{
						"type":        "string",
						"description": "Path to grant write access (must be a path returned by write tool that requires confirmation)",
					},
					"command": map[string]any{
						"type":        "string",
						"description": "Command name to grant execution access (must be a command returned by shell tool that requires confirmation)",
					},
				},
				"required": []string{},
			},
		),
	}
}

// Execute 执行工具
func (t *GrantPermissionTool) Execute(ctx context.Context, argStr string) (any, error) {
	t.LogStart()
	args, err := t.ParseArgs(argStr)
	if err != nil {
		return nil, err
	}

	sessionKey := sandbox.SessionKeyFromContext(ctx)
	if sessionKey == "" {
		return nil, fmt.Errorf("会话信息获取失败")
	}

	path, hasPath := args["path"].(string)
	command, hasCommand := args["command"].(string)

	if !hasPath && !hasCommand {
		return nil, fmt.Errorf("必须提供 path 或 command 参数")
	}

	var results []map[string]any

	if hasPath && path != "" {
		absPath, err := filepath.Abs(path)
		if err != nil {
			return nil, err
		}
		t.sandbox.Permissions().Grant(sessionKey, absPath)
		t.sandbox.LogAuditEvent(sandbox.AuditEventPermissionGrant, absPath, true, fmt.Sprintf("Session %s granted write access to %s", sessionKey, absPath))

		results = append(results, map[string]any{
			"type":    "path",
			"status":  "granted",
			"path":    absPath,
			"message": fmt.Sprintf("Write access granted to %s", absPath),
		})
	}

	if hasCommand && command != "" {
		t.sandbox.Permissions().GrantCommand(sessionKey, command)
		t.sandbox.LogAuditEvent(sandbox.AuditEventCommandPermissionGrant, command, true, fmt.Sprintf("Session %s granted permission to execute command %s", sessionKey, command))

		results = append(results, map[string]any{
			"type":    "command",
			"status":  "granted",
			"command": command,
			"message": fmt.Sprintf("Permission granted to execute command '%s'", command),
		})
	}

	if len(results) == 1 {
		return results[0], nil
	}
	return results, nil
}
