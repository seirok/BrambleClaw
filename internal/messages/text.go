package messages

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// TextMessage 文本消息
type TextMessage struct {
	ID        string            `json:"id"`
	Source    string            `json:"source"`
	Type      MessageType       `json:"type"`
	Content   string            `json:"content"`
	Metadata  map[string]string `json:"metadata,omitempty"`
	CreatedAt time.Time         `json:"created_at"`
}

// NewTextMessage 创建文本消息
func NewTextMessage(source, content string) *TextMessage {
	return &TextMessage{
		ID:        uuid.New().String(),
		Source:    source,
		Type:      MessageTypeText,
		Content:   content,
		Metadata:  make(map[string]string),
		CreatedAt: time.Now().UTC(),
	}
}

func (m *TextMessage) GetID() string                  { return m.ID }
func (m *TextMessage) GetSource() string              { return m.Source }
func (m *TextMessage) GetType() MessageType           { return m.Type }
func (m *TextMessage) GetCreatedAt() time.Time        { return m.CreatedAt }
func (m *TextMessage) GetMetadata() map[string]string { return m.Metadata }
func (m *TextMessage) ToText() string                 { return m.Content }
func (m *TextMessage) ToModelText() string            { return m.Content }

func (m *TextMessage) MarshalJSON() ([]byte, error) {
	type Alias TextMessage
	return json.Marshal((*Alias)(m))
}
