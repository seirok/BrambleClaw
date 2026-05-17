package tools

import (
	"context"
	"fmt"
	"neoclaw/internal/logger"
	"neoclaw/internal/sandbox"
)

// ListTool 列出目录内容工具
type ListTool struct {
	*BaseTool
}

// NewListTool 创建列出目录内容工具
func NewListTool(sandbox *sandbox.Sandbox) *ListTool {
	return &ListTool{
		BaseTool: NewBaseTool(
			"list",
			"List directory contents, protected by sandbox security mechanism",
			sandbox,
			map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"path": map[string]interface{}{
						"type":        "string",
						"description": "Directory path (relative to workspace or absolute path, optional, defaults to current directory)",
					},
				},
				"required": []string{},
			},
		),
	}
}

// Execute 执行工具
func (t *ListTool) Execute(ctx context.Context, argStr string) (interface{}, error) {
	t.LogStart()
	args, err := t.ParseArgs(argStr)
	if err != nil {
		return nil, err
	}

	path := "."
	if p, ok := args["path"].(string); ok && p != "" {
		path = p
	}

	resolvedPath := t.ResolvePath(path)
	return t.listFiles(ctx, resolvedPath)
}

// listFiles 列出目录内容
func (t *ListTool) listFiles(ctx context.Context, path string) (interface{}, error) {
	if t.sandbox == nil {
		return nil, fmt.Errorf("sandbox not configured")
	}

	if err := t.sandbox.ValidatePath(ctx, path, false); err != nil {
		logger.L().Error().Err(err).Str("path", path).Msg("ListTool: path validation failed")
		return nil, err
	}

	t.sandbox.LogAuditEvent(sandbox.AuditEventFileRead, path, true, "")
	t.sandbox.GetMetrics().IncrementFileOp()

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
