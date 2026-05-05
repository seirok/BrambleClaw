package messages

import "encoding/json"

// LLMMessage LLM 消息接口
type LLMMessage interface {
	Role() string
}

// SystemMessage 系统消息
type SystemMessage struct {
	Content string `json:"content"`
}

func (m SystemMessage) Role() string { return "system" }

// UserMessage 用户消息
type UserMessage struct {
	Content string `json:"content"`
	Source  string `json:"source,omitempty"`
}

func (m UserMessage) Role() string { return "user" }

// AssistantMessage 助手消息
type AssistantMessage struct {
	Content string `json:"content"`
}

func (m AssistantMessage) Role() string { return "assistant" }

// FunctionCall 工具函数调用
type FunctionCall struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}
