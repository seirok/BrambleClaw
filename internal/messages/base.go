package messages

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
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
	MessageTypeError       MessageType = "error"
)

// MessageBase 提供公共字段和基础方法实现
type MessageBase struct {
	ID        string               `json:"id"`
	Source    string               `json:"source"`
	Type      MessageType          `json:"type"`
	Metadata  map[string]string    `json:"metadata,omitempty"`
	CreatedAt time.Time            `json:"created_at"`
}

// NewMessageBase 创建新的 MessageBase（设置 ID、CreatedAt、初始化 Metadata）
func NewMessageBase(source string, msgType MessageType) MessageBase {
	return MessageBase{
		ID:        uuid.New().String(),
		Source:    source,
		Type:      msgType,
		Metadata:  make(map[string]string),
		CreatedAt: time.Now().UTC(),
	}
}

func (m *MessageBase) GetID() string               { return m.ID }
func (m *MessageBase) GetSource() string           { return m.Source }
func (m *MessageBase) GetType() MessageType        { return m.Type }
func (m *MessageBase) GetCreatedAt() time.Time     { return m.CreatedAt }
func (m *MessageBase) GetMetadata() map[string]string { return m.Metadata }

// MarshalJSON is provided for embedding structs that need it
func (m *MessageBase) MarshalJSON() ([]byte, error) {
	type Alias MessageBase
	return json.Marshal((*Alias)(m))
}

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
