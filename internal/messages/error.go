package messages

import (
	"encoding/json"
	"fmt"
	util "neoclaw/internal"
)

// AgentErrorMessage Agent错误消息
type AgentErrorMessage struct {
	MessageBase
	Error          string `json:"error"`
	ErrorType      string `json:"error_type,omitempty"`
	IsProgramError bool   `json:"is_program_error,omitempty"` // 是否是程序运行错误（类型2）
}

// NewAgentErrorMessage 创建Agent错误消息
func NewAgentErrorMessage(source, errMsg string) *AgentErrorMessage {
	return &AgentErrorMessage{
		MessageBase: NewMessageBase(source, MessageTypeError),
		Error:       errMsg,
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

func (m *AgentErrorMessage) ToText() string {
	if m.IsProgramError {
		return fmt.Sprintf("Some error happens. Please check the log under %s", util.GetLogPath())
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
