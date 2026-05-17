package tools

import (
	"context"
	"errors"
	"fmt"
	"neoclaw/internal/logger"
	"neoclaw/internal/sandbox"
	"os"
	"path/filepath"
)

// WriteTool 写入文件工具
type WriteTool struct {
	*BaseTool
}

// NewWriteTool 创建写入文件工具
func NewWriteTool(sandbox *sandbox.Sandbox) *WriteTool {
	return &WriteTool{
		BaseTool: NewBaseTool(
			"write",
			"写入文件内容，受沙箱安全机制保护",
			sandbox,
			map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"path": map[string]interface{}{
						"type":        "string",
						"description": "文件路径（相对于工作目录或绝对路径）",
					},
					"content": map[string]interface{}{
						"type":        "string",
						"description": "要写入的文件内容",
					},
					"append": map[string]interface{}{
						"type":        "boolean",
						"description": "是否追加写入（可选，默认 false）",
					},
				},
				"required": []string{"path", "content"},
			},
		),
	}
}

// Execute 执行工具
func (t *WriteTool) Execute(ctx context.Context, argStr string) (interface{}, error) {
	t.LogStart()
	args, err := t.ParseArgs(argStr)
	if err != nil {
		return nil, err
	}

	path, ok := args["path"].(string)
	if !ok {
		return nil, fmt.Errorf("路径参数类型错误")
	}
	content, ok := args["content"].(string)
	if !ok {
		return nil, fmt.Errorf("内容参数类型错误")
	}
	appendMode := false
	if appendVal, ok := args["append"].(bool); ok {
		appendMode = appendVal
	}

	resolvedPath := t.ResolvePath(path)
	return t.writeFile(ctx, resolvedPath, content, appendMode)
}

// writeFile 写入文件
func (t *WriteTool) writeFile(ctx context.Context, path string, content string, appendMode bool) (interface{}, error) {
	if t.sandbox == nil {
		return nil, fmt.Errorf("sandbox not configured")
	}
	if t.sandbox.Config() == nil {
		return nil, fmt.Errorf("sandbox config not available")
	}

	if err := t.sandbox.ValidatePath(ctx, path, true); err != nil {
		var pncErr *sandbox.PathNeedsConfirmationError
		if errors.As(err, &pncErr) {
			return map[string]interface{}{
				"status":      "needs_confirmation",
				"path":        pncErr.Path,
				"workspace":   pncErr.Workspace,
				"message":     fmt.Sprintf("需要用户确认才能写入 %s，该路径在工作目录外", pncErr.Path),
				"instruction": "请先询问用户是否允许写入该路径，用户同意后调用 grant_permission 工具授权，再重试写入。",
			}, nil
		}
		logger.L().Error().Err(err).Str("path", path).Msg("WriteTool: path validation failed")
		return nil, err
	}

	contentSize := int64(len(content))
	var totalSize int64 = contentSize
	if appendMode {
		if info, err := os.Stat(path); err == nil {
			totalSize = info.Size() + contentSize
		}
	}

	maxSize := t.sandbox.Config().FileSystem.MaxFileSize
	if maxSize > 0 && totalSize > maxSize {
		t.sandbox.LogAuditEvent(sandbox.AuditEventAccessDenied, path, false, "文件大小超出限制")
		return nil, fmt.Errorf("文件大小 %d 超过最大限制 %d", totalSize, maxSize)
	}

	t.sandbox.LogAuditEvent(sandbox.AuditEventFileWrite, path, true, "")
	t.sandbox.GetMetrics().IncrementFileOp()

	dir := filepath.Dir(path)
	if err := sandbox.EnsureDir(dir); err != nil {
		return nil, fmt.Errorf("创建目录失败(%s): %w", dir, err)
	}

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
