package agent

import "time"

// Role 消息角色
type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleSystem    Role = "system"
	RoleTool      Role = "tool"
)

// ContentBlock 内容块接口
type ContentBlock interface {
	Type() string
}

// TextContent 文本内容
type TextContent struct {
	Text string `json:"text"`
}

func (t TextContent) Type() string {
	return "text"
}

// AgentMessage Agent消息
type AgentMessage struct {
	Role      Role           `json:"role"`
	Content   []ContentBlock `json:"content"`
	Timestamp int64          `json:"timestamp"`
}

// Session 会话
type Session struct {
	Key       string         `json:"key"`
	Messages  []AgentMessage `json:"messages"` // 会话消息
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
}
