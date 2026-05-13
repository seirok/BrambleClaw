package audit

import (
	"brambleclaw/internal/config"
	"brambleclaw/internal/logger"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"gopkg.in/natefinch/lumberjack.v2"
)

const (
	defaultMaxParamSize  = 10 * 1024 // 10KB
	defaultMaxResultSize = 10 * 1024 // 10KB
)

// AuditEventType 审计事件类型
type AuditEventType string

const (
	// 文件操作事件
	AuditEventFileRead   AuditEventType = "FILE_READ"
	AuditEventFileWrite  AuditEventType = "FILE_WRITE"
	AuditEventFileDelete AuditEventType = "FILE_DELETE"
	AuditEventFileList   AuditEventType = "FILE_LIST"

	// 命令执行事件
	AuditEventCommandStart AuditEventType = "COMMAND_START"
	AuditEventCommandEnd   AuditEventType = "COMMAND_END"
	AuditEventCommandBlock AuditEventType = "COMMAND_BLOCK"

	// 安全事件
	AuditEventAccessDenied           AuditEventType = "ACCESS_DENIED"
	AuditEventPathEscape             AuditEventType = "PATH_ESCAPE"
	AuditEventTimeout                AuditEventType = "TIMEOUT"
	AuditEventPermissionGrant        AuditEventType = "PERMISSION_GRANT"
	AuditEventCommandPermissionGrant AuditEventType = "COMMAND_PERMISSION_GRANT"

	// 工具调用事件
	AuditEventToolCall AuditEventType = "TOOL_CALL"
)

// AuditEvent 审计事件
type AuditEvent struct {
	Timestamp time.Time      `json:"timestamp"`  // 事件时间戳
	EventType AuditEventType `json:"event_type"` // 事件类型
	SessionID string         `json:"session_id"` // 会话ID
	RequestID string         `json:"request_id"` // 请求ID

	// 操作信息
	Operation  string      `json:"operation"`  // 操作名称
	Target     string      `json:"target"`     // 操作目标（文件路径、命令、工具名等）
	Parameters interface{} `json:"parameters"` // 操作参数

	// 结果信息
	Success bool        `json:"success"` // 是否成功
	Result  interface{} `json:"result"`  // 操作结果
	Error   string      `json:"error"`   // 错误信息（如果有）

	// 上下文信息
	Workspace string `json:"workspace"`  // 工作目录
	AgentName string `json:"agent_name"` // Agent名称
	UserID    string `json:"user_id"`    // 用户ID
	Channel   string `json:"channel"`    // 来源通道

	// 性能指标
	Duration   time.Duration `json:"duration"`    // 操作耗时
	MemoryUsed int64         `json:"memory_used"` // 内存使用（字节）
}

// ToolCallAuditEvent 工具调用审计事件请求结构体
type ToolCallAuditEvent struct {
	ToolName   string
	Arguments  string
	Result     string
	Success    bool
	Error      string
	Duration   time.Duration
	SessionKey string
	Workspace  string
}

// AuditLogger 审计日志记录器
type AuditLogger struct {
	config    config.AuditConfig
	logger    *lumberjack.Logger
	mu        sync.RWMutex
	eventChan chan *AuditEvent
	stopChan  chan struct{}
	wg        sync.WaitGroup
}

// NewAuditLogger 创建审计日志记录器
func NewAuditLogger(config config.AuditConfig) (*AuditLogger, error) {
	if !config.Enabled {
		return &AuditLogger{config: config}, nil
	}

	logPath := config.LogPath
	if logPath == "" {
		logPath = "./logs/sandbox_audit.log"
	}

	// 确保日志目录存在
	logDir := filepath.Dir(logPath)
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, fmt.Errorf("创建日志目录失败(%s): %w", logDir, err)
	}

	logger := &lumberjack.Logger{
		Filename:   logPath,
		MaxSize:    config.MaxSize, // MB
		MaxBackups: config.MaxBackups,
		MaxAge:     30, // days
		Compress:   true,
	}

	al := &AuditLogger{
		config:    config,
		logger:    logger,
		eventChan: make(chan *AuditEvent, 1000),
		stopChan:  make(chan struct{}),
	}

	// 启动后台写入 goroutine
	al.wg.Add(1)
	go al.writeLoop()

	return al, nil
}

// NewAuditLoggerWithPath 使用指定 logPath 覆盖 cfg.LogPath 创建审计日志记录器
func NewAuditLoggerWithPath(config config.AuditConfig, logPath string) (*AuditLogger, error) {
	config.LogPath = logPath
	return NewAuditLogger(config)
}

// writeLoop 后台写入循环
func (al *AuditLogger) writeLoop() {
	defer al.wg.Done()

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case event := <-al.eventChan:
			al.writeEvent(event)
		case <-ticker.C:
			if al.logger != nil {
				al.logger.Close()
			}
		case <-al.stopChan:
			for {
				select {
				case event := <-al.eventChan:
					al.writeEvent(event)
				default:
					return
				}
			}
		}
	}
}

// writeEvent 写入单个事件
func (al *AuditLogger) writeEvent(event *AuditEvent) {
	if al.logger == nil {
		return
	}

	data, err := json.Marshal(event)
	if err != nil {
		logger.L().Error().Err(err).Msg("Failed to serialize audit event")
		return
	}

	al.mu.Lock()
	fmt.Fprintln(al.logger, string(data))
	al.mu.Unlock()
}

// Log 记录审计事件
func (al *AuditLogger) Log(event *AuditEvent) {
	if !al.config.Enabled {
		return
	}

	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	select {
	case al.eventChan <- event:
	default:
		logger.L().Warn().Str("event_type", string(event.EventType)).Msg("Audit event channel full, dropping event")
	}
}

// LogToolCall 记录工具调用审计事件
func (al *AuditLogger) LogToolCall(tc *ToolCallAuditEvent) {
	if !al.config.Enabled {
		return
	}

	agentName, channel := parseSessionKey(tc.SessionKey)

	event := &AuditEvent{
		Timestamp:  time.Now(),
		EventType:  AuditEventToolCall,
		SessionID:  tc.SessionKey,
		Target:     tc.ToolName,
		Parameters: truncateForAudit(tc.Arguments, al.getOrDefault(al.config.MaxParamSize, defaultMaxParamSize)),
		Result:     truncateForAudit(tc.Result, al.getOrDefault(al.config.MaxResultSize, defaultMaxResultSize)),
		Success:    tc.Success,
		Error:      tc.Error,
		Duration:   tc.Duration,
		Workspace:  tc.Workspace,
		AgentName:  agentName,
		Channel:    channel,
	}

	al.Log(event)
}

// Close 关闭审计日志记录器
func (al *AuditLogger) Close() error {
	if !al.config.Enabled {
		return nil
	}

	close(al.stopChan)
	al.wg.Wait()

	if al.logger != nil {
		return al.logger.Close()
	}

	return nil
}

// truncateForAudit 截断审计日志字符串
func truncateForAudit(s string, maxLen int) string {
	if maxLen <= 0 {
		maxLen = defaultMaxResultSize
	}
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "...[truncated]"
}

// getOrDefault 返回值或默认值
func (al *AuditLogger) getOrDefault(val, def int) int {
	if val <= 0 {
		return def
	}
	return val
}

// parseSessionKey 解析 session key 为 agent name 和 channel
func parseSessionKey(sessionKey string) (agentName, channel string) {
	parts := strings.SplitN(sessionKey, "::", 3)
	if len(parts) >= 2 {
		channel = parts[0]
		agentName = parts[1]
	}
	return
}
