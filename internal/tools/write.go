package tools

import (
	"brambleclaw/internal/logger"
	"brambleclaw/internal/sandbox"
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
)

// WriteTool 写入文件工具
type WriteTool struct {
	sandbox *sandbox.Sandbox
	name    string
	desc    string
}

// NewWriteTool 创建写入文件工具
func NewWriteTool(sandbox *sandbox.Sandbox) *WriteTool {
	return &WriteTool{
		sandbox: sandbox,
		name:    "write",
		desc:    "写入文件内容，受沙箱安全机制保护",
	}
}

// Name 返回工具名称
func (t *WriteTool) Name() string {
	return t.name
}

// Description 返回工具描述
func (t *WriteTool) Description() string {
	return t.desc
}

// Parameters 返回工具参数定义
func (t *WriteTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "文件路径（相对于工作目录或绝对路径）",
			},
			"content": map[string]any{
				"type":        "string",
				"description": "要写入的文件内容",
			},
			"append": map[string]any{
				"type":        "boolean",
				"description": "是否追加写入（可选，默认 false）",
			},
		},
		"required": []string{"path", "content"},
	}
}

// Execute 执行工具
func (t *WriteTool) Execute(ctx context.Context, argStr string) (any, error) {
	// 解析参数
	var args map[string]any
	if err := json.Unmarshal([]byte(argStr), &args); err != nil {
		return nil, fmt.Errorf("解析参数失败: %w", err)
	}

	// 获取路径
	path, ok := args["path"].(string)
	if !ok {
		return nil, fmt.Errorf("路径参数类型错误")
	}

	// 获取内容
	content, ok := args["content"].(string)
	if !ok {
		return nil, fmt.Errorf("内容参数类型错误")
	}

	// 获取 append 选项
	appendMode := false
	if appendVal, ok := args["append"].(bool); ok {
		appendMode = appendVal
	}

	// 处理路径
	resolvedPath := t.resolvePath(path)

	// 写入文件
	return t.writeFile(ctx, resolvedPath, content, appendMode)
}

// resolvePath 解析路径
func (t *WriteTool) resolvePath(path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	if t.sandbox != nil && t.sandbox.Config() != nil {
		return filepath.Join(t.sandbox.Config().Workspace, path)
	}
	return path
}

// writeFile 写入文件
func (t *WriteTool) writeFile(ctx context.Context, path string, content string, appendMode bool) (any, error) {
	// 验证路径
	if err := t.sandbox.ValidatePath(path, true); err != nil {
		logger.L().Error().Err(err).Str("path", path).Msg("WriteTool: path validation failed")
		return nil, err
	}

	// 检查文件大小限制
	contentSize := int64(len(content))
	if t.sandbox.Config().FileSystem.MaxFileSize > 0 && contentSize > t.sandbox.Config().FileSystem.MaxFileSize {
		t.sandbox.LogAuditEvent(sandbox.AuditEventAccessDenied, path, false, "文件大小超出限制")
		return nil, fmt.Errorf("文件大小 %d 超过最大限制 %d", contentSize, t.sandbox.Config().FileSystem.MaxFileSize)
	}

	// 记录审计日志
	t.sandbox.LogAuditEvent(sandbox.AuditEventFileWrite, path, true, "")
	t.sandbox.GetMetrics().IncrementFileOp()

	// 确保目录存在
	dir := filepath.Dir(path)
	if err := sandbox.EnsureDir(dir); err != nil {
		return nil, fmt.Errorf("创建目录失败(%s): %w", dir, err)
	}

	// 写入文件
	var err error
	if appendMode {
		err = sandbox.AppendFileWithContext(ctx, path, []byte(content), 0644)
	} else {
		err = sandbox.WriteFileWithContext(ctx, path, []byte(content), 0644)
	}
	if err != nil {
		return nil, fmt.Errorf("写入文件失败(%s): %w", path, err)
	}

	return "File written successfully", nil
}
