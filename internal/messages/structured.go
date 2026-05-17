package messages

import (
	"encoding/json"
)

// StructuredMessage 结构化消息，支持泛型内容
type StructuredMessage[T any] struct {
	MessageBase
	Content T `json:"content"`
}

// NewStructuredMessage 创建结构化消息
func NewStructuredMessage[T any](source string, content T) *StructuredMessage[T] {
	return &StructuredMessage[T]{
		MessageBase: NewMessageBase(source, MessageTypeStructured),
		Content:     content,
	}
}

func (m *StructuredMessage[T]) ToText() string {
	data, _ := json.Marshal(m.Content)
	return string(data)
}

func (m *StructuredMessage[T]) ToModelText() string {
	return m.ToText()
}

func (m *StructuredMessage[T]) MarshalJSON() ([]byte, error) {
	type Alias StructuredMessage[T]
	return json.Marshal((*Alias)(m))
}
