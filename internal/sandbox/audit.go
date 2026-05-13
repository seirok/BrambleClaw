package sandbox

import auditpkg "brambleclaw/internal/audit"

// AuditEventType 审计事件类型
type AuditEventType = auditpkg.AuditEventType

const (
	// 文件操作事件
	AuditEventFileRead   AuditEventType = auditpkg.AuditEventFileRead
	AuditEventFileWrite  AuditEventType = auditpkg.AuditEventFileWrite
	AuditEventFileDelete AuditEventType = auditpkg.AuditEventFileDelete
	AuditEventFileList   AuditEventType = auditpkg.AuditEventFileList

	// 命令执行事件
	AuditEventCommandStart AuditEventType = auditpkg.AuditEventCommandStart
	AuditEventCommandEnd   AuditEventType = auditpkg.AuditEventCommandEnd
	AuditEventCommandBlock AuditEventType = auditpkg.AuditEventCommandBlock

	// 安全事件
	AuditEventAccessDenied           AuditEventType = auditpkg.AuditEventAccessDenied
	AuditEventPathEscape             AuditEventType = auditpkg.AuditEventPathEscape
	AuditEventTimeout                AuditEventType = auditpkg.AuditEventTimeout
	AuditEventPermissionGrant        AuditEventType = auditpkg.AuditEventPermissionGrant
	AuditEventCommandPermissionGrant AuditEventType = auditpkg.AuditEventCommandPermissionGrant

	// 工具调用事件
	AuditEventToolCall AuditEventType = auditpkg.AuditEventToolCall
)

// AuditEvent 审计事件
type AuditEvent = auditpkg.AuditEvent

// AuditLogger 审计日志记录器
type AuditLogger = auditpkg.AuditLogger

// NewAuditLogger 创建审计日志记录器
var NewAuditLogger = auditpkg.NewAuditLogger
