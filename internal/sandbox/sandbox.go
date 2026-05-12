package sandbox

import (
	"brambleclaw/internal/config"
	"brambleclaw/internal/hook"
	"brambleclaw/internal/logger"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Sandbox 沙箱
type Sandbox struct {
	config      *config.SandboxConfig
	auditLogger *AuditLogger
	mu          sync.RWMutex
	metrics     *Metrics
}

// Metrics 沙箱指标
type Metrics struct {
	FileOperations    int64         `json:"file_operations"`
	CommandExecutions int64         `json:"command_executions"`
	BlockedOperations int64         `json:"blocked_operations"`
	TotalDuration     time.Duration `json:"total_duration"`
	mu                sync.RWMutex
}

func NewSandbox(config *config.SandboxConfig, auditLogger *AuditLogger) (*Sandbox, error) {
	if config == nil {
		return nil, fmt.Errorf("沙箱配置不能为空")
	}

	// 验证工作目录
	if config.Workspace == "" {
		return nil, fmt.Errorf("工作目录不能为空")
	}

	// 确保工作目录存在
	if err := ensureDir(config.Workspace); err != nil {
		return nil, fmt.Errorf("创建工作目录失败(%s): %w", config.Workspace, err)
	}

	s := &Sandbox{
		config:      config,
		auditLogger: auditLogger,
		metrics:     &Metrics{},
	}

	return s, nil
}

// IsEnabled 检查沙箱是否启用
func (s *Sandbox) IsEnabled() bool {
	if s == nil || s.config == nil {
		return false
	}
	return s.config.Enabled
}

// ValidatePath 验证路径是否在允许范围内
func (s *Sandbox) ValidatePath(path string, forWrite bool) error {
	if !s.IsEnabled() {
		return nil
	}

	// 解析绝对路径
	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("解析路径失败(%s): %w", path, err)
	}

	// 触发路径验证钩子
	data := map[string]interface{}{
		"path":          path,
		"absolute_path": absPath,
		"for_write":     forWrite,
		"workspace":     s.config.Workspace,
	}
	if processedData, err := hook.Emit(context.Background(), "hook.point.sandbox.path.validate", data); err != nil {
		return fmt.Errorf("路径验证钩子拒绝访问: %w", err)
	} else if processedData != nil {
		// 钩子可能修改了路径
		if newPath, ok := processedData.(string); ok {
			path = newPath
			// 重新解析绝对路径
			newAbsPath, err := filepath.Abs(path)
			if err != nil {
				return fmt.Errorf("解析修改后的路径失败(%s): %w", path, err)
			}
			absPath = newAbsPath
		}
	}

	// 检查是否在工作目录内
	workspaceAbs, _ := filepath.Abs(s.config.Workspace)
	rel, err := filepath.Rel(workspaceAbs, absPath)
	if err != nil {
		return fmt.Errorf("计算相对路径失败: %w", err)
	}

	// 检查路径是否被允许
	if strings.HasPrefix(rel, "..") {
		// 路径在工作目录外
		if forWrite {
			// 写入操作：检查是否在 AllowWritePaths 中
			if s.isPathInAllowWritePathsList(absPath) {
				s.logAuditEvent(AuditEventPathEscape, absPath, true, "允许写入配置的外部路径")
				return nil
			}
			// 不在允许列表中，拒绝
			s.logAuditEvent(AuditEventPathEscape, absPath, false, "路径逃逸被拒绝")
			s.metrics.IncrementBlocked()
			return fmt.Errorf("路径逃逸: %s 超出工作目录 %s", absPath, workspaceAbs)
		} else {
			// 读取操作：检查 AllowReadOutside
			if s.config.AllowReadOutside {
				s.logAuditEvent(AuditEventPathEscape, absPath, true, "允许读取工作目录外的文件")
				return nil
			}
			// 不允许读取外部文件
			s.logAuditEvent(AuditEventPathEscape, absPath, false, "路径逃逸被拒绝")
			s.metrics.IncrementBlocked()
			return fmt.Errorf("路径逃逸: %s 超出工作目录 %s", absPath, workspaceAbs)
		}
	}

	// 路径在工作目录内：总是允许
	return nil
}

// 检查路径是否在允许的写入路径列表中（仅检查 AllowWritePaths，不包含工作目录）
func (s *Sandbox) isPathInAllowWritePathsList(path string) bool {
	for _, pattern := range s.config.FileSystem.AllowWritePaths {
		matched, err := regexp.MatchString(pattern, path)
		if err == nil && matched {
			return true
		}
	}
	return false
}

// ValidateCommand 验证命令是否在白名单中
func (s *Sandbox) ValidateCommand(command string) error {
	if !s.IsEnabled() {
		return nil
	}

	// 提取命令名称（去除参数）
	cmdName := extractCommandName(command)

	// 触发命令验证钩子
	data := map[string]interface{}{
		"command":     command,
		"cmd_name":    cmdName,
		"working_dir": s.config.Workspace,
	}
	if processedData, err := hook.Emit(context.Background(), "hook.point.sandbox.command.validate", data); err != nil {
		return fmt.Errorf("命令验证钩子拒绝执行: %w", err)
	} else if processedData != nil {
		// 钩子可能修改了命令
		if newCommand, ok := processedData.(string); ok {
			command = newCommand
			cmdName = extractCommandName(command)
		}
	}

	// 检查是否在白名单中
	if !IsCommandAllowed(s.config, cmdName) {
		s.logAuditEvent(AuditEventCommandBlock, command, false, "命令不在白名单中")
		s.metrics.IncrementBlocked()
		return fmt.Errorf("命令 '%s' 不在允许的白名单中", cmdName)
	}

	s.logAuditEvent(AuditEventCommandStart, command, true, "命令验证通过")
	logger.L().Debug().Str("command", command).Msg("validation pass")
	return nil
}

// ExecuteCommand 在沙箱中执行命令
func (s *Sandbox) ExecuteCommand(ctx context.Context, command string) (string, error) {
	if !s.IsEnabled() {
		// 沙箱未启用，直接执行
		return s.executeRaw(ctx, command)
	}

	// 验证命令
	if err := s.ValidateCommand(command); err != nil {
		return "", err
	}

	// 创建带超时的上下文
	timeout := s.config.Execution.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// 记录开始时间
	startTime := time.Now()

	// 执行命令
	output, err := s.executeRaw(ctx, command)

	// 记录执行结果
	duration := time.Since(startTime)
	s.metrics.AddDuration(duration)

	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			s.logAuditEvent(AuditEventTimeout, command, false, fmt.Sprintf("命令执行超时（%v）", timeout))
			s.metrics.IncrementBlocked()
			return "", fmt.Errorf("命令执行超时（限制: %v）", timeout)
		}
		s.logAuditEvent(AuditEventCommandEnd, command, false, fmt.Sprintf("执行失败: %v", err))
		return output, fmt.Errorf("命令执行失败: %w", err)
	}

	s.logAuditEvent(AuditEventCommandEnd, command, true, fmt.Sprintf("执行成功，耗时: %v", duration))
	return output, nil
}

// executeRaw 直接执行命令（不经过沙箱检查）
func (s *Sandbox) executeRaw(ctx context.Context, command string) (string, error) {
	// 使用 powershell 执行命令（Windows）
	cmd := exec.CommandContext(ctx, "powershell", "-NoProfile", "-Command", command)

	// 设置工作目录
	if s.config != nil && s.config.Workspace != "" {
		cmd.Dir = s.config.Workspace
	}

	output, err := cmd.CombinedOutput()
	return string(output), err
}

// extractCommandName 提取命令名称
func extractCommandName(command string) string {
	// 去除首尾空白
	command = strings.TrimSpace(command)

	// 获取第一个单词（命令名称）
	parts := strings.Fields(command)
	if len(parts) == 0 {
		return ""
	}

	// 去除可能的引号
	cmd := strings.Trim(parts[0], `"'`)

	// 如果包含路径，只返回文件名
	return filepath.Base(cmd)
}

// logAuditEvent 记录审计事件
func (s *Sandbox) logAuditEvent(eventType AuditEventType, target string, success bool, message string) {
	if s.auditLogger == nil {
		return
	}

	event := &AuditEvent{
		Timestamp: time.Now(),
		EventType: eventType,
		Target:    target,
		Success:   success,
		Error:     message,
	}

	s.auditLogger.Log(event)
}

// Config 返回沙箱配置
func (s *Sandbox) Config() *config.SandboxConfig {
	return s.config
}

// LogAuditEvent 记录审计事件
func (s *Sandbox) LogAuditEvent(eventType AuditEventType, target string, success bool, message string) {
	s.logAuditEvent(eventType, target, success, message)
}

// GetMetrics 获取沙箱指标
func (s *Sandbox) GetMetrics() *Metrics {
	s.metrics.mu.RLock()
	defer s.metrics.mu.RUnlock()

	// 返回副本
	return &Metrics{
		FileOperations:    s.metrics.FileOperations,
		CommandExecutions: s.metrics.CommandExecutions,
		BlockedOperations: s.metrics.BlockedOperations,
		TotalDuration:     s.metrics.TotalDuration,
	}
}

// 指标操作方法

func (m *Metrics) IncrementFileOp() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.FileOperations++
}

func (m *Metrics) IncrementCommand() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.CommandExecutions++
}

func (m *Metrics) IncrementBlocked() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.BlockedOperations++
}

func (m *Metrics) AddDuration(d time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.TotalDuration += d
}

// ensureDir 确保目录存在
func ensureDir(path string) error {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return os.MkdirAll(path, 0755)
	}
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("路径不是目录: %s", path)
	}
	return nil
}
