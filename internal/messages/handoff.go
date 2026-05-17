package messages

import (
	"encoding/json"
)

// HandoffMessage 移交消息，用于将对话移交给另一个 Agent
type HandoffMessage struct {
	MessageBase
	Target  string `json:"target"`
	Context string `json:"context,omitempty"`
}

// NewHandoffMessage 创建移交消息
func NewHandoffMessage(source, target, context string) *HandoffMessage {
	return &HandoffMessage{
		MessageBase: NewMessageBase(source, MessageTypeHandoff),
		Target:      target,
		Context:     context,
	}
}

func (m *HandoffMessage) ToText() string      { return m.Context }
func (m *HandoffMessage) ToModelText() string { return m.Context }

func (m *HandoffMessage) MarshalJSON() ([]byte, error) {
	type Alias HandoffMessage
	return json.Marshal((*Alias)(m))
}
