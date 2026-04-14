package sandbox

import (
	"brambleclaw/logger"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// FileSystemTool 带沙箱的文件系统工具
type FileSystemTool struct {
	sandbox *Sandbox
	name    string
	desc    string
}

// NewFileSystemTool 创建带沙箱的文件系统工具
func NewFileSystemTool(sandbox *Sandbox) *FileSystemTool {
	return &FileSystemTool{
		sandbox: sandbox,
		name:    "FileSystem",
		desc:    "带沙箱保护的文件系统操作工具，用于读写工作目录内的文件",
	}
}

// Name 返回工具名称
func (t *FileSystemTool) Name() string {
	return t.name
}

// Description 返回工具描述
func (t *FileSystemTool) Description() string {
	return t.desc
}

// Parameters 返回工具参数定义
func (t *FileSystemTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"command": map[string]interface{}{
				"type":        "string",
				"description": "操作命令: read, write, list, delete",
				"enum":        []string{"read", "write", "list", "delete"},
			},
			"path": map[string]interface{}{
				"type":        "string",
				"description": "文件或目录路径（相对于工作目录或绝对路径）",
			},
			"content": map[string]interface{}{
				"type":        "string",
				"description": "文件内容（仅write命令需要）",
			},
		},
		"required": []string{"command", "path"},
	}
}

// Execute 执行工具
func (t *FileSystemTool) Execute(ctx context.Context, argStr string) (interface{}, error) {
	// 解析参数
	var args map[string]interface{}
	if err := json.Unmarshal([]byte(argStr), &args); err != nil {
		return nil, fmt.Errorf("解析参数失败: %w", err)
	}

	// 获取命令
	cmd, ok := args["command"].(string)
	if !ok {
		return nil, fmt.Errorf("命令参数类型错误")
	}

	// 获取路径
	path, ok := args["path"].(string)
	if !ok {
		return nil, fmt.Errorf("路径参数类型错误")
	}

	// 处理路径（解析相对路径）
	resolvedPath := t.resolvePath(path)

	// 根据命令执行相应操作
	switch cmd {
	case "read":
		return t.readFile(ctx, resolvedPath)
	case "write":
		content, _ := args["content"].(string)
		return t.writeFile(ctx, resolvedPath, content)
	case "list":
		return t.listDirectory(ctx, resolvedPath)
	case "delete":
		return t.deleteFile(ctx, resolvedPath)
	default:
		return nil, fmt.Errorf("未知的命令: %s", cmd)
	}
}

// resolvePath 解析路径
func (t *FileSystemTool) resolvePath(path string) string {
	// 如果是绝对路径，直接使用
	if filepath.IsAbs(path) {
		return path
	}

	// 如果是相对路径，基于工作目录解析
	if t.sandbox != nil && t.sandbox.config != nil {
		return filepath.Join(t.sandbox.config.Workspace, path)
	}

	// 默认返回原路径
	return path
}

// readFile 读取文件
func (t *FileSystemTool) readFile(ctx context.Context, path string) (interface{}, error) {
	// 验证路径
	if err := t.sandbox.ValidatePath(path, false); err != nil {
		t.logAudit(AuditEventAccessDenied, path, false, err.Error())
		return nil, err
	}

	// 记录审计日志
	t.logAudit(AuditEventFileRead, path, true, "")
	t.sandbox.metrics.IncrementFileOp()

	// 读取文件
	data, err := readFileWithContext(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("读取文件失败(%s): %w", path, err)
	}

	return string(data), nil
}

// writeFile 写入文件
func (t *FileSystemTool) writeFile(ctx context.Context, path string, content string) (interface{}, error) {
	// 验证路径
	if err := t.sandbox.ValidatePath(path, true); err != nil {
		t.logAudit(AuditEventAccessDenied, path, false, err.Error())
		return nil, err
	}

	// 检查文件大小限制
	contentSize := int64(len(content))
	if t.sandbox.config.FileSystem.MaxFileSize > 0 && contentSize > t.sandbox.config.FileSystem.MaxFileSize {
		t.logAudit(AuditEventAccessDenied, path, false, "文件大小超出限制")
		return nil, fmt.Errorf("文件大小 %d 超过最大限制 %d", contentSize, t.sandbox.config.FileSystem.MaxFileSize)
	}

	// 记录审计日志
	t.logAudit(AuditEventFileWrite, path, true, "")
	t.sandbox.metrics.IncrementFileOp()

	// 确保目录存在
	dir := filepath.Dir(path)
	if err := ensureDir(dir); err != nil {
		return nil, fmt.Errorf("创建目录失败(%s): %w", dir, err)
	}

	// 写入文件
	if err := writeFileWithContext(ctx, path, []byte(content), 0644); err != nil {
		return nil, fmt.Errorf("写入文件失败(%s): %w", path, err)
	}

	return "File written successfully", nil
}

// listDirectory 列出目录内容
func (t *FileSystemTool) listDirectory(ctx context.Context, path string) (interface{}, error) {
	// 验证路径
	if err := t.sandbox.ValidatePath(path, false); err != nil {
		t.logAudit(AuditEventAccessDenied, path, false, err.Error())
		return nil, err
	}

	// 记录审计日志
	t.logAudit(AuditEventFileList, path, true, "")
	t.sandbox.metrics.IncrementFileOp()

	// 读取目录
	entries, err := listDirWithContext(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("读取目录失败(%s): %w", path, err)
	}

	// 构建文件列表
	fileList := make([]map[string]interface{}, 0, len(entries))
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			continue
		}

		fileList = append(fileList, map[string]interface{}{
			"name":     info.Name(),
			"size":     info.Size(),
			"is_dir":   info.IsDir(),
			"mod_time": info.ModTime().Format(time.RFC3339),
		})
	}

	return fileList, nil
}

// deleteFile 删除文件
func (t *FileSystemTool) deleteFile(ctx context.Context, path string) (interface{}, error) {
	// 验证路径
	if err := t.sandbox.ValidatePath(path, true); err != nil {
		t.logAudit(AuditEventAccessDenied, path, false, err.Error())
		return nil, err
	}

	// 记录审计日志
	t.logAudit(AuditEventFileDelete, path, true, "")
	t.sandbox.metrics.IncrementFileOp()

	// 删除文件或目录
	if err := deleteWithContext(ctx, path); err != nil {
		return nil, fmt.Errorf("删除失败(%s): %w", path, err)
	}

	return "File deleted successfully", nil
}

// logAudit 记录审计日志
func (t *FileSystemTool) logAudit(eventType AuditEventType, target string, success bool, message string) {
	if t.sandbox == nil || t.sandbox.auditLogger == nil {
		return
	}

	event := &AuditEvent{
		Timestamp: time.Now(),
		EventType: eventType,
		Target:    target,
		Success:   success,
		Error:     message,
	}

	t.sandbox.auditLogger.Log(event)
}

// Helper 函数

func readFileWithContext(ctx context.Context, path string) ([]byte, error) {
	type result struct {
		data []byte
		err  error
	}

	ch := make(chan result, 1)
	go func() {
		data, err := readFileLimited(path, 10*1024*1024) // 10MB limit
		ch <- result{data, err}
	}()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case res := <-ch:
		return res.data, res.err
	}
}

func writeFileWithContext(ctx context.Context, path string, data []byte, perm uint32) error {
	type result struct {
		err error
	}

	ch := make(chan result, 1)
	go func() {
		err := writeFileLimited(path, data, perm, 10*1024*1024) // 10MB limit
		ch <- result{err}
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case res := <-ch:
		return res.err
	}
}

func listDirWithContext(ctx context.Context, path string) ([]os.DirEntry, error) {
	type result struct {
		entries []os.DirEntry
		err     error
	}

	ch := make(chan result, 1)
	go func() {
		entries, err := os.ReadDir(path)
		ch <- result{entries, err}
	}()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case res := <-ch:
		return res.entries, res.err
	}
}

func deleteWithContext(ctx context.Context, path string) error {
	type result struct {
		err error
	}

	ch := make(chan result, 1)
	go func() {
		err := os.RemoveAll(path)
		ch <- result{err}
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case res := <-ch:
		return res.err
	}
}

// 带大小限制的文件读取
func readFileLimited(path string, maxSize int64) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	if info.Size() > maxSize {
		return nil, fmt.Errorf("文件大小 %d 超过最大限制 %d", info.Size(), maxSize)
	}

	return os.ReadFile(path)
}

// 带大小限制的文件写入
func writeFileLimited(path string, data []byte, perm uint32, maxSize int64) error {
	logger.L().Debug().Str("path", path).Msg("writeFileLimited: begin to write limited file")
	if int64(len(data)) > maxSize {
		return fmt.Errorf("写入数据大小 %d 超过最大限制 %d", len(data), maxSize)
	}

	return os.WriteFile(path, data, os.FileMode(perm))
}
