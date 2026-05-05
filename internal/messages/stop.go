package messages

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// StopMessage 停止消息，用于终止对话
type StopMessage struct {
	ID        string            `json:"id"`
	Source    string            `json:"source"`
	Type      MessageType       `json:"type"`
	Reason    string            `json:"reason"`
	Metadata  map[string]string `json:"metadata,omitempty"`
	CreatedAt time.Time         `json:"created_at"`
}

// NewStopMessage 创建停止消息
func NewStopMessage(source, reason string) *StopMessage {
	return &StopMessage{
		ID:        uuid.New().String(),
		Source:    source,
		Type:      MessageTypeStop,
		Reason:    reason,
		Metadata:  make(map[string]string),
		CreatedAt: time.Now().UTC(),
	}
}

func (m *StopMessage) GetID() string                  { return m.ID }
func (m *StopMessage) GetSource() string              { return m.Source }
func (m *StopMessage) GetType() MessageType           { return m.Type }
func (m *StopMessage) GetCreatedAt() time.Time        { return m.CreatedAt }
func (m *StopMessage) GetMetadata() map[string]string { return m.Metadata }
func (m *StopMessage) ToText() string                 { return m.Reason }
func (m *StopMessage) ToModelText() string            { return m.Reason }

func (m *StopMessage) MarshalJSON() ([]byte, error) {
	type Alias StopMessage
	return json.Marshal((*Alias)(m))
}
