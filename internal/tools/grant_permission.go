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
		desc:    "授予临时权限（仅当前会话有效）。可授权写入工作目录外的路径，或执行不在白名单中的命令。只有在用户明确同意后才能调用此工具。",
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
				"description": "要授权写入的路径（必须是 write 工具返回的需要确认的路径）",
			},
			"command": map[string]interface{}{
				"type":        "string",
				"description": "要授权执行的命令名称（必须是 shell 工具返回的需要确认的命令）",
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
		return nil, fmt.Errorf("解析参数失败: %w", err)
	}

	sessionKey := sandbox.SessionKeyFromContext(ctx)
	if sessionKey == "" {
		return nil, fmt.Errorf("无法获取会话信息，授权失败")
	}

	path, hasPath := args["path"].(string)
	command, hasCommand := args["command"].(string)

	if !hasPath && !hasCommand {
		return nil, fmt.Errorf("必须提供 path 或 command 参数")
	}

	var results []map[string]interface{}

	// 授权路径写入
	if hasPath && path != "" {
		absPath, err := filepath.Abs(path)
		if err != nil {
			return nil, fmt.Errorf("解析路径失败: %w", err)
		}

		t.sandbox.Permissions().Grant(sessionKey, absPath)
		t.sandbox.LogAuditEvent(sandbox.AuditEventPermissionGrant, absPath, true,
			fmt.Sprintf("会话 %s 已授权写入 %s", sessionKey, absPath))

		results = append(results, map[string]interface{}{
			"type":    "path",
			"status":  "granted",
			"path":    absPath,
			"message": fmt.Sprintf("已授权写入 %s，可以重试写入操作", absPath),
		})
	}

	// 授权命令执行
	if hasCommand && command != "" {
		t.sandbox.Permissions().GrantCommand(sessionKey, command)
		t.sandbox.LogAuditEvent(sandbox.AuditEventCommandPermissionGrant, command, true,
			fmt.Sprintf("会话 %s 已授权执行命令 %s", sessionKey, command))

		results = append(results, map[string]interface{}{
			"type":    "command",
			"status":  "granted",
			"command": command,
			"message": fmt.Sprintf("已授权执行命令 '%s'，可以重试执行", command),
		})
	}

	// 单个授权直接返回对象，多个授权返回数组
	if len(results) == 1 {
		return results[0], nil
	}
	return results, nil
}
