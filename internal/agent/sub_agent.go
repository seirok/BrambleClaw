package agent

import (
	"brambleclaw/internal/interfaces"
	"brambleclaw/internal/messages"
	"brambleclaw/internal/tools"
	"context"
	"fmt"
)

// SubAgent 二级 Agent，拥有独立 LLM 能力但无会话持久化
// 由一级 Agent 通过 create_team 工具创建，仅在 team 任务期间存活
type SubAgent struct {
	*BaseChatAgent
	orche        *Orchestrator
	builder      *SubContextBuilder
	messages     []AgentMessage
	systemPrompt string
}

// NewSubAgent 创建二级 Agent
func NewSubAgent(
	name, description, systemPrompt string,
	llm LLMProcessor,
	parentTools interfaces.Registry[tools.Tool],
	toolNames []string,
) (*SubAgent, error) {
	subTools := tools.NewToolRegistry()
	for _, n := range toolNames {
		tool, err := parentTools.Get(context.Background(), n)
		if err != nil {
			return nil, fmt.Errorf("sub-agent %q: tool %q not found in parent: %w", name, n, err)
		}
		if err := subTools.Register(context.Background(), n, tool); err != nil {
			return nil, fmt.Errorf("sub-agent %q: failed to register tool %q: %w", name, n, err)
		}
	}

	builder := NewSubContextBuilder(name, description, systemPrompt, subTools)
	orche := NewOrchestrator(llm, subTools)

	return &SubAgent{
		BaseChatAgent: NewBaseChatAgent(name, description),
		orche:         orche,
		builder:       builder,
		messages:      make([]AgentMessage, 0),
	}, nil
}

func (s *SubAgent) ProducedMessageTypes() []messages.MessageType {
	return []messages.MessageType{
		messages.MessageTypeText,
		messages.MessageTypeToolCallSum,
	}
}

// OnMessages 处理消息，通过 Orchestrator 调用 LLM
// 无会话持久化，消息在内存中累积直到 SubAgent 被回收
func (s *SubAgent) OnMessages(ctx context.Context, msgs []messages.ChatMessage) (*Response, error) {
	if s.systemPrompt == "" {
		prompt, err := s.builder.Build()
		if err != nil {
			return nil, fmt.Errorf("sub-agent %q: build context failed: %w", s.Name(), err)
		}
		s.systemPrompt = prompt
		s.messages = append(s.messages, *NewAgentMessage(s.Name(), RoleSystem, s.systemPrompt))
	}

	for _, msg := range msgs {
		s.messages = append(s.messages, *NewAgentMessage(msg.GetSource(), RoleUser, msg.ToModelText()))
	}

	history := make([]messages.BaseMessage, len(s.messages))
	for i, m := range s.messages {
		history[i] = &m
	}

	resp, err := s.orche.Run(ctx, history)
	if err != nil {
		return nil, fmt.Errorf("sub-agent %q: orchestrator run failed: %w", s.Name(), err)
	}

	replyContent := ""
	if len(resp.Choices) > 0 {
		replyContent = resp.Choices[0].Message.Content
	}

	s.messages = append(s.messages, *NewAgentMessage(s.Name(), RoleAssistant, replyContent))

	return &Response{
		ChatMessage: messages.NewTextMessage(s.Name(), replyContent),
	}, nil
}

func (s *SubAgent) OnMessagesStream(ctx context.Context, msgs []messages.ChatMessage) (<-chan StreamItem, error) {
	return streamFromOnMessages(ctx, msgs, s.OnMessages), nil
}

// 编译时检查：确保 SubAgent 实现了 ChatAgent 接口
var _ ChatAgent = (*SubAgent)(nil)
