package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"neoclaw/internal/logger"
	"neoclaw/internal/sandbox"
	"path/filepath"
)

// ListTool 列出目录内容工具
type ListTool struct {
	sandbox *sandbox.Sandbox
	name    string
	desc    string
}

// NewListTool 创建列出目录内容工具
func NewListTool(sandbox *sandbox.Sandbox) *ListTool {
	return &ListTool{
		sandbox: sandbox,
		name:    "list",
		desc:    "List directory contents, protected by sandbox security mechanism",
	}
}

// Name 返回工具名称
func (t *ListTool) Name() string {
	return t.name
}

// Description 返回工具描述
func (t *ListTool) Description() string {
	return t.desc
}

// Parameters 返回工具参数定义
func (t *ListTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "Directory path (relative to workspace or absolute path, optional, defaults to current directory)",
			},
		},
		"required": []string{},
	}
}

// Execute 执行工具
func (t *ListTool) Execute(ctx context.Context, argStr string) (interface{}, error) {
	logger.L().Info().Str("tool", t.name).Msg("tool start to execute")
	// 解析参数
	var args map[string]interface{}
	if err := json.Unmarshal([]byte(argStr), &args); err != nil {
		return nil, fmt.Errorf("Failed to parse parameters: %w", err)
	}

	// 获取路径
	path := "."
	if p, ok := args["path"].(string); ok && p != "" {
		path = p
	}

	// 处理路径
	resolvedPath := t.resolvePath(path)

	// 列出目录
	return t.listFiles(ctx, resolvedPath)
}

// resolvePath 解析路径
func (t *ListTool) resolvePath(path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	if t.sandbox != nil && t.sandbox.Config() != nil {
		return filepath.Join(t.sandbox.Config().Workspace, path)
	}
	return path
}

// listFiles 列出目录内容
func (t *ListTool) listFiles(ctx context.Context, path string) (interface{}, error) {
	// 验证 sandbox 不为 nil
	if t.sandbox == nil {
		return nil, fmt.Errorf("sandbox not configured")
	}

	// 验证路径
	if err := t.sandbox.ValidatePath(ctx, path, false); err != nil {
		logger.L().Error().Err(err).Str("path", path).Msg("ListTool: path validation failed")
		return nil, err
	}

	// 记录审计日志
	t.sandbox.LogAuditEvent(sandbox.AuditEventFileRead, path, true, "")
	t.sandbox.GetMetrics().IncrementFileOp()

	// 列出目录
	files, err := sandbox.ReadDirWithContext(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("Failed to read directory(%s): %w", path, err)
	}

	fileList := make([]map[string]interface{}, 0, len(files))
	for _, file := range files {
		info, err := file.Info()
		if err != nil {
			logger.L().Error().Err(err).Str("File", file.Name()).Msg("Failed to get file info")
			continue
		}
		fileList = append(fileList, map[string]interface{}{
			"name":     info.Name(),
			"size":     info.Size(),
			"is_dir":   info.IsDir(),
			"mod_time": info.ModTime(),
		})
	}

	return fileList, nil
}
