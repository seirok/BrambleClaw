package messages

import (
	"encoding/json"
)

// StopMessage 停止消息，用于终止对话
type StopMessage struct {
	MessageBase
	Reason string `json:"reason"`
}

// NewStopMessage 创建停止消息
func NewStopMessage(source, reason string) *StopMessage {
	return &StopMessage{
		MessageBase: NewMessageBase(source, MessageTypeStop),
		Reason:      reason,
	}
}

func (m *StopMessage) ToText() string      { return m.Reason }
func (m *StopMessage) ToModelText() string { return m.Reason }

func (m *StopMessage) MarshalJSON() ([]byte, error) {
	type Alias StopMessage
	return json.Marshal((*Alias)(m))
}
