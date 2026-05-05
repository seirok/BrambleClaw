package messages

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// ToolCallRequestEvent 工具调用请求事件
type ToolCallRequestEvent struct {
	ID        string            `json:"id"`
	Source    string            `json:"source"`
	Type      MessageType       `json:"type"`
	ToolCalls []FunctionCall    `json:"tool_calls"`
	Metadata  map[string]string `json:"metadata,omitempty"`
	CreatedAt time.Time         `json:"created_at"`
}

// NewToolCallRequestEvent 创建工具调用请求事件
func NewToolCallRequestEvent(source string, toolCalls []FunctionCall) *ToolCallRequestEvent {
	return &ToolCallRequestEvent{
		ID:        uuid.New().String(),
		Source:    source,
		Type:      MessageTypeToolCallSum,
		ToolCalls: toolCalls,
		Metadata:  make(map[string]string),
		CreatedAt: time.Now().UTC(),
	}
}

func (m *ToolCallRequestEvent) GetID() string                  { return m.ID }
func (m *ToolCallRequestEvent) GetSource() string              { return m.Source }
func (m *ToolCallRequestEvent) GetType() MessageType           { return m.Type }
func (m *ToolCallRequestEvent) GetCreatedAt() time.Time        { return m.CreatedAt }
func (m *ToolCallRequestEvent) GetMetadata() map[string]string { return m.Metadata }

func (m *ToolCallRequestEvent) ToText() string {
	data, _ := json.Marshal(m.ToolCalls)
	return string(data)
}

func (m *ToolCallRequestEvent) MarshalJSON() ([]byte, error) {
	type Alias ToolCallRequestEvent
	return json.Marshal((*Alias)(m))
}

// ToolCallExecutionEvent 工具调用执行结果事件
type ToolCallExecutionEvent struct {
	ID        string            `json:"id"`
	Source    string            `json:"source"`
	Type      MessageType       `json:"type"`
	Results   []ToolCallResult  `json:"results"`
	Metadata  map[string]string `json:"metadata,omitempty"`
	CreatedAt time.Time         `json:"created_at"`
}

// ToolCallResult 单个工具调用结果
type ToolCallResult struct {
	ToolCallID string `json:"tool_call_id"`
	Content    string `json:"content"`
}

// NewToolCallExecutionEvent 创建工具调用执行结果事件
func NewToolCallExecutionEvent(source string, results []ToolCallResult) *ToolCallExecutionEvent {
	return &ToolCallExecutionEvent{
		ID:        uuid.New().String(),
		Source:    source,
		Type:      MessageTypeToolCallSum,
		Results:   results,
		Metadata:  make(map[string]string),
		CreatedAt: time.Now().UTC(),
	}
}

func (m *ToolCallExecutionEvent) GetID() string                  { return m.ID }
func (m *ToolCallExecutionEvent) GetSource() string              { return m.Source }
func (m *ToolCallExecutionEvent) GetType() MessageType           { return m.Type }
func (m *ToolCallExecutionEvent) GetCreatedAt() time.Time        { return m.CreatedAt }
func (m *ToolCallExecutionEvent) GetMetadata() map[string]string { return m.Metadata }

func (m *ToolCallExecutionEvent) ToText() string {
	data, _ := json.Marshal(m.Results)
	return string(data)
}

func (m *ToolCallExecutionEvent) MarshalJSON() ([]byte, error) {
	type Alias ToolCallExecutionEvent
	return json.Marshal((*Alias)(m))
}
