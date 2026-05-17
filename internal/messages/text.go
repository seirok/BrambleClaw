package messages

import (
	"encoding/json"
)

// TextMessage 文本消息
type TextMessage struct {
	MessageBase
	Content string `json:"content"`
}

// NewTextMessage 创建文本消息
func NewTextMessage(source, content string) *TextMessage {
	return &TextMessage{
		MessageBase: NewMessageBase(source, MessageTypeText),
		Content:     content,
	}
}

func (m *TextMessage) ToText() string      { return m.Content }
func (m *TextMessage) ToModelText() string { return m.Content }

func (m *TextMessage) MarshalJSON() ([]byte, error) {
	type Alias TextMessage
	return json.Marshal((*Alias)(m))
}
