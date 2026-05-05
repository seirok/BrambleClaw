package messages

import "time"

// MessageType 消息类型
type MessageType string

const (
	MessageTypeText        MessageType = "text"
	MessageTypeMultiModal  MessageType = "multimodal"
	MessageTypeStop        MessageType = "stop"
	MessageTypeHandoff     MessageType = "handoff"
	MessageTypeToolCallSum MessageType = "tool_call_summary"
	MessageTypeStructured  MessageType = "structured"
)

// BaseMessage 所有消息的基础接口
type BaseMessage interface {
	GetID() string
	GetSource() string
	GetType() MessageType
	GetCreatedAt() time.Time
	GetMetadata() map[string]string
	ToText() string
	MarshalJSON() ([]byte, error)
}

// ChatMessage Agent 间通信消息接口，扩展 BaseMessage
type ChatMessage interface {
	BaseMessage
	ToModelText() string
}

// AgentEvent 事件通知接口，用于事件广播而非 Agent 间通信
type AgentEvent interface {
	BaseMessage
}
