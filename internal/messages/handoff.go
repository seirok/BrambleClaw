package messages

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// HandoffMessage 移交消息，用于将对话移交给另一个 Agent
type HandoffMessage struct {
	ID        string            `json:"id"`
	Source    string            `json:"source"`
	Type      MessageType       `json:"type"`
	Target    string            `json:"target"`
	Context   string            `json:"context,omitempty"`
	Metadata  map[string]string `json:"metadata,omitempty"`
	CreatedAt time.Time         `json:"created_at"`
}

// NewHandoffMessage 创建移交消息
func NewHandoffMessage(source, target, context string) *HandoffMessage {
	return &HandoffMessage{
		ID:        uuid.New().String(),
		Source:    source,
		Type:      MessageTypeHandoff,
		Target:    target,
		Context:   context,
		Metadata:  make(map[string]string),
		CreatedAt: time.Now().UTC(),
	}
}

func (m *HandoffMessage) GetID() string                  { return m.ID }
func (m *HandoffMessage) GetSource() string              { return m.Source }
func (m *HandoffMessage) GetType() MessageType           { return m.Type }
func (m *HandoffMessage) GetCreatedAt() time.Time        { return m.CreatedAt }
func (m *HandoffMessage) GetMetadata() map[string]string { return m.Metadata }
func (m *HandoffMessage) ToText() string                 { return m.Context }
func (m *HandoffMessage) ToModelText() string            { return m.Context }

func (m *HandoffMessage) MarshalJSON() ([]byte, error) {
	type Alias HandoffMessage
	return json.Marshal((*Alias)(m))
}
