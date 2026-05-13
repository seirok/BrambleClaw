package sandbox

import (
	"brambleclaw/internal/config"
	"brambleclaw/internal/logger"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"gopkg.in/natefinch/lumberjack.v2"
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
)

// AuditEvent 审计事件
type AuditEvent struct {
	Timestamp time.Time      `json:"timestamp"`  // 事件时间戳
	EventType AuditEventType `json:"event_type"` // 事件类型
	SessionID string         `json:"session_id"` // 会话ID
	RequestID string         `json:"request_id"` // 请求ID

	// 操作信息
	Operation  string      `json:"operation"`  // 操作名称
	Target     string      `json:"target"`     // 操作目标（文件路径、命令等）
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

	// 确保日志目录存在
	logDir := filepath.Dir(config.LogPath)
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, fmt.Errorf("创建日志目录失败(%s): %w", logDir, err)
	}

	logger := &lumberjack.Logger{
		Filename:   config.LogPath,
		MaxSize:    config.MaxSize, // MB
		MaxBackups: config.MaxBackups,
		MaxAge:     30, // days
		Compress:   true,
	}

	al := &AuditLogger{
		config:    config,
		logger:    logger,
		eventChan: make(chan *AuditEvent, 1000), // 缓冲通道
		stopChan:  make(chan struct{}),
	}

	// 启动后台写入 goroutine
	al.wg.Add(1)
	go al.writeLoop()

	return al, nil
}

// writeLoop 后台写入循环
func (al *AuditLogger) writeLoop() {
	defer al.wg.Done()

	ticker := time.NewTicker(5 * time.Second) // 定期刷新
	defer ticker.Stop()

	for {
		select {
		case event := <-al.eventChan:
			al.writeEvent(event)

		case <-ticker.C:
			// 定期刷新缓冲区
			if al.logger != nil {
				al.logger.Close()
			}

		case <-al.stopChan:
			// 处理剩余的通道事件
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

	// 序列化为 JSON
	data, err := json.Marshal(event)
	if err != nil {
		logger.L().Error().Err(err).Msg("Failed to serialize audit event")
		return
	}

	// 写入日志文件（每行一个 JSON 对象）
	al.mu.Lock()
	fmt.Fprintln(al.logger, string(data))
	al.mu.Unlock()
}

// Log 记录审计事件
func (al *AuditLogger) Log(event *AuditEvent) {
	if !al.config.Enabled {
		return
	}

	// 设置时间戳（如果未设置）
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	// 发送到通道（非阻塞）
	select {
	case al.eventChan <- event:
		// 成功发送
	default:
		// 通道已满，记录丢弃事件
		logger.L().Warn().Str("event_type", string(event.EventType)).Msg("Audit event channel full, dropping event")
	}
}

// LogAsync 异步记录审计事件（带回调）
func (al *AuditLogger) LogAsync(event *AuditEvent, callback func(error)) {
	if !al.config.Enabled {
		if callback != nil {
			callback(nil)
		}
		return
	}

	// 在 goroutine 中处理
	go func() {
		al.Log(event)
		if callback != nil {
			callback(nil)
		}
	}()
}

// Close 关闭审计日志记录器
func (al *AuditLogger) Close() error {
	if !al.config.Enabled {
		return nil
	}

	// 发送停止信号
	close(al.stopChan)

	// 等待写入完成
	al.wg.Wait()

	// 关闭日志文件
	if al.logger != nil {
		return al.logger.Close()
	}

	return nil
}

// Query 查询审计日志（简化实现）
func (al *AuditLogger) Query(startTime, endTime time.Time, eventType AuditEventType) ([]*AuditEvent, error) {
	// TODO: 实现日志查询功能
	// 这里可以实现读取日志文件并解析 JSON 的功能
	return nil, fmt.Errorf("查询功能尚未实现")
}

// GetStats 获取审计统计信息
func (al *AuditLogger) GetStats() map[string]interface{} {
	return map[string]interface{}{
		"enabled":     al.config.Enabled,
		"log_path":    al.config.LogPath,
		"buffer_size": cap(al.eventChan),
	}
}
