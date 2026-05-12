package tools

import (
	"brambleclaw/internal/logger"
	"brambleclaw/internal/sandbox"
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
)

// ReadTool 读取文件工具
type ReadTool struct {
	sandbox *sandbox.Sandbox
	name    string
	desc    string
}

// NewReadTool 创建读取文件工具
func NewReadTool(sandbox *sandbox.Sandbox) *ReadTool {
	return &ReadTool{
		sandbox: sandbox,
		name:    "read",
		desc:    "读取文件内容，受沙箱安全机制保护",
	}
}

// Name 返回工具名称
func (t *ReadTool) Name() string {
	return t.name
}

// Description 返回工具描述
func (t *ReadTool) Description() string {
	return t.desc
}

// Parameters 返回工具参数定义
func (t *ReadTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
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
	}
}

// Execute 执行工具
func (t *ReadTool) Execute(ctx context.Context, argStr string) (interface{}, error) {
	// 解析参数
	var args map[string]interface{}
	if err := json.Unmarshal([]byte(argStr), &args); err != nil {
		return nil, fmt.Errorf("解析参数失败: %w", err)
	}

	// 获取路径
	path, ok := args["path"].(string)
	if !ok {
		return nil, fmt.Errorf("路径参数类型错误")
	}

	// 处理路径
	resolvedPath := t.resolvePath(path)

	// 读取文件
	return t.readFile(ctx, resolvedPath, args)
}

// resolvePath 解析路径
func (t *ReadTool) resolvePath(path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	if t.sandbox != nil && t.sandbox.Config() != nil {
		return filepath.Join(t.sandbox.Config().Workspace, path)
	}
	return path
}

// readFile 读取文件
func (t *ReadTool) readFile(ctx context.Context, path string, args map[string]interface{}) (interface{}, error) {
	// 验证 sandbox 不为 nil
	if t.sandbox == nil {
		return nil, fmt.Errorf("sandbox not configured")
	}

	// 验证路径
	if err := t.sandbox.ValidatePath(path, false); err != nil {
		logger.L().Error().Err(err).Str("path", path).Msg("ReadTool: path validation failed")
		return nil, err
	}

	// 记录审计日志
	t.sandbox.LogAuditEvent(sandbox.AuditEventFileRead, path, true, "")
	t.sandbox.GetMetrics().IncrementFileOp()

	// 读取文件
	data, err := sandbox.ReadFileWithContext(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("读取文件失败(%s): %w", path, err)
	}

	// 处理 offset 和 limit
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
