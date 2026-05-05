package messages

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// StructuredMessage 结构化消息，支持泛型内容
type StructuredMessage[T any] struct {
	ID        string            `json:"id"`
	Source    string            `json:"source"`
	Type      MessageType       `json:"type"`
	Content   T                 `json:"content"`
	Metadata  map[string]string `json:"metadata,omitempty"`
	CreatedAt time.Time         `json:"created_at"`
}

// NewStructuredMessage 创建结构化消息
func NewStructuredMessage[T any](source string, content T) *StructuredMessage[T] {
	return &StructuredMessage[T]{
		ID:        uuid.New().String(),
		Source:    source,
		Type:      MessageTypeStructured,
		Content:   content,
		Metadata:  make(map[string]string),
		CreatedAt: time.Now().UTC(),
	}
}

func (m *StructuredMessage[T]) GetID() string                  { return m.ID }
func (m *StructuredMessage[T]) GetSource() string              { return m.Source }
func (m *StructuredMessage[T]) GetType() MessageType           { return m.Type }
func (m *StructuredMessage[T]) GetCreatedAt() time.Time        { return m.CreatedAt }
func (m *StructuredMessage[T]) GetMetadata() map[string]string { return m.Metadata }

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
