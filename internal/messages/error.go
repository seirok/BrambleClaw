package messages

import (
	util "brambleclaw/internal"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// AgentErrorMessage Agent错误消息
type AgentErrorMessage struct {
	ID            string            `json:"id"`
	Source        string            `json:"source"`
	Type          MessageType       `json:"type"`
	Error         string            `json:"error"`
	ErrorType     string            `json:"error_type,omitempty"`
	IsProgramError bool             `json:"is_program_error,omitempty"` // 是否是程序运行错误（类型2）
	Metadata      map[string]string `json:"metadata,omitempty"`
	CreatedAt     time.Time         `json:"created_at"`
}

// NewAgentErrorMessage 创建Agent错误消息
func NewAgentErrorMessage(source, errMsg string) *AgentErrorMessage {
	return &AgentErrorMessage{
		ID:            uuid.NewString(),
		Source:        source,
		Type:          MessageTypeError,
		Error:         errMsg,
		Metadata:      make(map[string]string),
		CreatedAt:     time.Now().UTC(),
	}
}

// WithErrorType 设置错误类型（可选）
func (m *AgentErrorMessage) WithErrorType(errType string) *AgentErrorMessage {
	m.ErrorType = errType
	return m
}

// WithIsProgramError 设置是否是程序运行错误（类型2）
func (m *AgentErrorMessage) WithIsProgramError(isProgramError bool) *AgentErrorMessage {
	m.IsProgramError = isProgramError
	return m
}

func (m *AgentErrorMessage) GetID() string                  { return m.ID }
func (m *AgentErrorMessage) GetSource() string              { return m.Source }
func (m *AgentErrorMessage) GetType() MessageType           { return m.Type }
func (m *AgentErrorMessage) GetCreatedAt() time.Time        { return m.CreatedAt }
func (m *AgentErrorMessage) GetMetadata() map[string]string { return m.Metadata }
func (m *AgentErrorMessage) ToText() string {
	if m.IsProgramError {
		return fmt.Sprintf("Some error happened. Please check the log under %s", util.GetLogPath())
	}
	return m.Error
}
func (m *AgentErrorMessage) ToModelText() string {
	return fmt.Sprintf("ERROR from %s: %s", m.Source, m.Error)
}

func (m *AgentErrorMessage) MarshalJSON() ([]byte, error) {
	type Alias AgentErrorMessage
	return json.Marshal((*Alias)(m))
}
