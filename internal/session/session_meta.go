package session

import "time"

type SessionMetadata struct {
	AgentName        string    `json:"agent_name"`
	ChannelName      string    `json:"channel_name"`
	ChatID           string    `json:"chat_id"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
	MessageCount     int       `json:"message_count"`
	TokenCount       int       `json:"token_count"`
	SessionSummary   string    `json:"session_summary,omitempty"` // 会话摘要（多条，带时间戳）
	FirstUserMessage string    `json:"first_user_message,omitempty"`
}

// NewSessionMetadata 创建新的 SessionMetadata 实例（使用 functional options 模式）
func NewSessionMetadata(opts ...MetadataOption) *SessionMetadata {
	m := &SessionMetadata{
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	// 应用所有选项
	for _, opt := range opts {
		opt(m)
	}

	return m
}

// MetadataOption 定义 SessionMetadata 的 functional option 类型
type MetadataOption func(*SessionMetadata)

// WithAgentName 设置 agent 名称
func WithAgentName(name string) MetadataOption {
	return func(m *SessionMetadata) {
		m.AgentName = name
	}
}

// WithChannelName 设置 channel 名称
func WithChannelName(name string) MetadataOption {
	return func(m *SessionMetadata) {
		m.ChannelName = name
	}
}

// WithChatID 设置 chat ID
func WithChatID(id string) MetadataOption {
	return func(m *SessionMetadata) {
		m.ChatID = id
	}
}

// WithMetadataCreatedAt 设置创建时间
func WithMetadataCreatedAt(t time.Time) MetadataOption {
	return func(m *SessionMetadata) {
		m.CreatedAt = t
	}
}

// WithMetadataUpdatedAt 设置更新时间
func WithMetadataUpdatedAt(t time.Time) MetadataOption {
	return func(m *SessionMetadata) {
		m.UpdatedAt = t
	}
}

// WithMessageCount 设置消息数量
func WithMessageCount(count int) MetadataOption {
	return func(m *SessionMetadata) {
		m.MessageCount = count
	}
}

// WithTokenCount 设置 token 数量
func WithTokenCount(count int) MetadataOption {
	return func(m *SessionMetadata) {
		m.TokenCount = count
	}
}

// WithSessionSummary 设置会话摘要
func WithSessionSummary(summary string) MetadataOption {
	return func(m *SessionMetadata) {
		m.SessionSummary = summary
	}
}

// WithFirstUserMessage 设置第一条用户消息
func WithFirstUserMessage(msg string) MetadataOption {
	return func(m *SessionMetadata) {
		m.FirstUserMessage = msg
	}
}
