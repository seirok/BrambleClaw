package messages

import (
	"encoding/json"
)

// ToolCallRequestEvent 工具调用请求事件
type ToolCallRequestEvent struct {
	MessageBase
	ToolCalls []FunctionCall `json:"tool_calls"`
}

// NewToolCallRequestEvent 创建工具调用请求事件
func NewToolCallRequestEvent(source string, toolCalls []FunctionCall) *ToolCallRequestEvent {
	return &ToolCallRequestEvent{
		MessageBase: NewMessageBase(source, MessageTypeToolCallSum),
		ToolCalls:   toolCalls,
	}
}

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
	MessageBase
	Results []ToolCallResult `json:"results"`
}

// ToolCallResult 单个工具调用结果
type ToolCallResult struct {
	ToolCallID string `json:"tool_call_id"`
	Content    string `json:"content"`
}

// NewToolCallExecutionEvent 创建工具调用执行结果事件
func NewToolCallExecutionEvent(source string, results []ToolCallResult) *ToolCallExecutionEvent {
	return &ToolCallExecutionEvent{
		MessageBase: NewMessageBase(source, MessageTypeToolCallSum),
		Results:     results,
	}
}

func (m *ToolCallExecutionEvent) ToText() string {
	data, _ := json.Marshal(m.Results)
	return string(data)
}

func (m *ToolCallExecutionEvent) MarshalJSON() ([]byte, error) {
	type Alias ToolCallExecutionEvent
	return json.Marshal((*Alias)(m))
}
