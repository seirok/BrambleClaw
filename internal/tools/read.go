package tools

import (
	"context"
	"fmt"
	"neoclaw/internal/logger"
	"neoclaw/internal/sandbox"
)

// ReadTool 读取文件工具
type ReadTool struct {
	*BaseTool
}

// NewReadTool 创建读取文件工具
func NewReadTool(sandbox *sandbox.Sandbox) *ReadTool {
	return &ReadTool{
		BaseTool: NewBaseTool(
			"read",
			"读取文件内容，受沙箱安全机制保护",
			sandbox,
			map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"path": map[string]interface{}{
						"type":        "string",
						"description": "文件路径（相对于工作目录或绝对路径）",
					},
					"offset": map[string]interface{}{
						"type":        "integer",
						"description": "读取偏移量（字节，可选）",
					},
					"limit": map[string]interface{}{
						"type":        "integer",
						"description": "读取字节数限制（可选）",
					},
				},
				"required": []string{"path"},
			},
		),
	}
}

// Execute 执行工具
func (t *ReadTool) Execute(ctx context.Context, argStr string) (interface{}, error) {
	t.LogStart()
	args, err := t.ParseArgs(argStr)
	if err != nil {
		return nil, err
	}

	path, ok := args["path"].(string)
	if !ok {
		return nil, fmt.Errorf("路径参数类型错误")
	}

	resolvedPath := t.ResolvePath(path)
	return t.readFile(ctx, resolvedPath, args)
}

// readFile 读取文件
func (t *ReadTool) readFile(ctx context.Context, path string, args map[string]interface{}) (interface{}, error) {
	if t.sandbox == nil {
		return nil, fmt.Errorf("sandbox not configured")
	}

	if err := t.sandbox.ValidatePath(ctx, path, false); err != nil {
		logger.L().Error().Err(err).Str("path", path).Msg("ReadTool: path validation failed")
		return nil, err
	}

	t.sandbox.LogAuditEvent(sandbox.AuditEventFileRead, path, true, "")
	t.sandbox.GetMetrics().IncrementFileOp()

	data, err := sandbox.ReadFileWithContext(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("读取文件失败(%s): %w", path, err)
	}

	content := string(data)
	if offsetVal, ok := args["offset"].(float64); ok {
		offset := int(offsetVal)
		if offset > 0 && offset < len(content) {
			content = content[offset:]
		} else if offset >= len(content) {
			content = ""
		}
	}
	if limitVal, ok := args["limit"].(float64); ok {
		limit := int(limitVal)
		if limit > 0 && limit < len(content) {
			content = content[:limit]
		}
	}

	return content, nil
}
