package messages

import (
	"context"
	"time"
)

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

// RuntimeProvider 定义运行时发布/订阅的最小接口
type RuntimeProvider interface {
	Publish(ctx context.Context, topicID string, msg BaseMessage)
	Subscribe(ctx context.Context, topicID string) RuntimeSubscription
}

// RuntimeSubscription 运行时订阅句柄的最小接口
type RuntimeSubscription interface {
	Ch() <-chan BaseMessage
	Cancel()
}
