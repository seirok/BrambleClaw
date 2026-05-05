package agent

import (
	"brambleclaw/internal/messages"
	"context"
	"sync/atomic"
)

// Response Agent 响应
type Response struct {
	ChatMessage   messages.ChatMessage
	InnerMessages []messages.BaseMessage
}

// StreamItem 流式输出项
type StreamItem struct {
	Message  messages.BaseMessage
	Response *Response
	Err      error
}

// ChatAgent Agent 间通信的核心接口
type ChatAgent interface {
	Name() string
	Description() string
	ProducedMessageTypes() []messages.MessageType
	OnMessages(ctx context.Context, msgs []messages.ChatMessage) (*Response, error)
	OnMessagesStream(ctx context.Context, msgs []messages.ChatMessage) (<-chan StreamItem, error)
	OnReset(ctx context.Context) error
	OnPause(ctx context.Context) error
	OnResume(ctx context.Context) error
	Close() error
	SaveState() (map[string]any, error)
	LoadState(state map[string]any) error
}

// BaseChatAgent 可嵌入的 Agent 基础实现，提供默认生命周期方法
type BaseChatAgent struct {
	name        string
	description string
	isPaused    atomic.Bool
}

// NewBaseChatAgent 创建基础 Agent
func NewBaseChatAgent(name, description string) *BaseChatAgent {
	return &BaseChatAgent{
		name:        name,
		description: description,
	}
}

func (b *BaseChatAgent) Name() string        { return b.name }
func (b *BaseChatAgent) Description() string { return b.description }

func (b *BaseChatAgent) OnReset(ctx context.Context) error    { return nil }
func (b *BaseChatAgent) OnPause(ctx context.Context) error    { b.isPaused.Store(true); return nil }
func (b *BaseChatAgent) OnResume(ctx context.Context) error   { b.isPaused.Store(false); return nil }
func (b *BaseChatAgent) Close() error                         { return nil }
func (b *BaseChatAgent) SaveState() (map[string]any, error)   { return nil, nil }
func (b *BaseChatAgent) LoadState(state map[string]any) error { return nil }

// IsPaused 检查是否暂停
func (b *BaseChatAgent) IsPaused() bool {
	return b.isPaused.Load()
}
