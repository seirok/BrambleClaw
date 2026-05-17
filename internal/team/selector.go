package team

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"neoclaw/internal/agent"
	"neoclaw/internal/logger"
	"neoclaw/internal/messages"
)

const DefaultMaxHistory = 20

type SelectorManager struct {
	ctx                 GroupChatManagerContext
	conversationHistory []messages.ChatMessage
	maxHistory          int
	currentIndex        int
	mu                  sync.Mutex
	cancel              context.CancelFunc
}

func NewSelectorManager() *SelectorManager {
	return &SelectorManager{
		maxHistory: DefaultMaxHistory,
	}
}

func (m *SelectorManager) Initialize(_ context.Context, context GroupChatManagerContext) error {
	m.ctx = context
	m.conversationHistory = make([]messages.ChatMessage, 0)
	m.currentIndex = 0
	return nil
}

func (m *SelectorManager) Start(ctx context.Context) error {
	mgrCtx, cancel := context.WithCancel(ctx)
	m.cancel = cancel

	mgrSub := m.ctx.Runtime.Subscribe(mgrCtx, m.ctx.ManagerTopic)
	groupSub := m.ctx.Runtime.Subscribe(mgrCtx, m.ctx.GroupTopic)

	go func() {
		for {
			select {
			case <-mgrCtx.Done():
				return
			case msg, ok := <-mgrSub.Ch():
				if !ok {
					return
				}
				if chatMsg, ok := msg.(messages.ChatMessage); ok {
					m.handleTask(mgrCtx, chatMsg)
				}
			case msg, ok := <-groupSub.Ch():
				if !ok {
					return
				}
				if chatMsg, ok := msg.(messages.ChatMessage); ok {
					m.handleResponse(mgrCtx, chatMsg)
				}
			}
		}
	}()

	return nil
}

func (m *SelectorManager) Pause(_ context.Context) error { return nil }

func (m *SelectorManager) Resume(_ context.Context) error { return nil }

func (m *SelectorManager) Reset(_ context.Context) error {
	m.mu.Lock()
	m.conversationHistory = make([]messages.ChatMessage, 0)
	m.currentIndex = 0
	m.mu.Unlock()
	return nil
}

func (m *SelectorManager) Stop() {
	if m.cancel != nil {
		m.cancel()
	}
}

func (m *SelectorManager) SelectAndPublish(_ context.Context, msg messages.ChatMessage) error {
	idx, err := m.selectNext()
	if err != nil {
		logger.L().Warn().Err(err).Msg("Selector LLM failed, falling back to round-robin")
		m.mu.Lock()
		idx = m.currentIndex
		m.currentIndex = (m.currentIndex + 1) % len(m.ctx.Participants)
		m.mu.Unlock()
	}
	m.ctx.Runtime.Publish(context.Background(), m.ctx.ParticipantTopics[idx], msg)
	return nil
}

func (m *SelectorManager) selectNext() (int, error) {
	if m.ctx.LLM == nil {
		return 0, fmt.Errorf("selector: no LLM client configured")
	}

	systemPrompt := m.buildSystemPrompt()
	userPrompt := m.buildUserPrompt()

	req := agent.ChatCompletionRequest{
		Model: m.ctx.LLM.Model(),
		Messages: []agent.ChatMsg{
			{Role: agent.RoleSystem, Content: systemPrompt},
			{Role: agent.RoleUser, Content: userPrompt},
		},
	}

	resp, err := m.ctx.LLM.Chat(req)
	if err != nil {
		return 0, fmt.Errorf("selector LLM call failed: %w", err)
	}

	if len(resp.Choices) == 0 {
		return 0, fmt.Errorf("selector LLM returned no choices")
	}

	selectedName := strings.TrimSpace(resp.Choices[0].Message.Content)
	idx := m.findParticipant(selectedName)
	if idx < 0 {
		return 0, fmt.Errorf("selector LLM chose unknown participant: %q", selectedName)
	}

	m.mu.Lock()
	m.currentIndex = (idx + 1) % len(m.ctx.Participants)
	m.mu.Unlock()

	return idx, nil
}

func (m *SelectorManager) buildSystemPrompt() string {
	var sb strings.Builder
	sb.WriteString("You are a conversation facilitator. Your job is to select the next speaker in a group chat.\n")
	sb.WriteString("The participants are:\n")
	for _, p := range m.ctx.Participants {
		fmt.Fprintf(&sb, "- %s: %s\n", p.Name(), p.Description())
	}
	sb.WriteString("\nRespond with ONLY the name of the participant who should speak next. No explanation, no punctuation, just the name.")
	return sb.String()
}

func (m *SelectorManager) buildUserPrompt() string {
	var sb strings.Builder

	m.mu.Lock()
	historyLen := len(m.conversationHistory)
	if historyLen > 0 {
		start := 0
		if historyLen > m.maxHistory {
			start = historyLen - m.maxHistory
		}
		sb.WriteString("Conversation so far:\n")
		for _, msg := range m.conversationHistory[start:] {
			fmt.Fprintf(&sb, "[%s]: %s\n", msg.GetSource(), msg.ToText())
		}
		sb.WriteString("\n")
	}
	m.mu.Unlock()

	sb.WriteString("Select the next speaker.")
	return sb.String()
}

func (m *SelectorManager) findParticipant(name string) int {
	nameLower := strings.ToLower(name)
	for i, p := range m.ctx.Participants {
		if strings.ToLower(p.Name()) == nameLower || strings.Contains(strings.ToLower(p.Name()), nameLower) || strings.Contains(nameLower, strings.ToLower(p.Name())) {
			return i
		}
	}
	return -1
}

func (m *SelectorManager) handleTask(ctx context.Context, msg messages.ChatMessage) {
	m.mu.Lock()
	m.conversationHistory = append(m.conversationHistory, msg)
	m.mu.Unlock()

	m.ctx.Runtime.Publish(ctx, m.ctx.OutputTopic, msg)
	m.SelectAndPublish(ctx, msg)
}

func (m *SelectorManager) handleResponse(ctx context.Context, msg messages.ChatMessage) {
	m.mu.Lock()
	m.conversationHistory = append(m.conversationHistory, msg)
	m.mu.Unlock()

	m.ctx.Runtime.Publish(ctx, m.ctx.OutputTopic, msg)

	if messages.IsStopMessage(msg) {
		return
	}

	if messages.IsErrorMessage(msg) {
		switch m.ctx.ErrorPolicy {
		case ErrorPolicyTerminate:
			stopMsg := messages.NewStopMessage(
				"manager",
				"team terminated due to error from "+msg.GetSource()+": "+messages.GetErrorDetail(msg),
			)
			m.ctx.Runtime.Publish(ctx, m.ctx.OutputTopic, stopMsg)
			return
		case ErrorPolicySkip:
			logger.L().Warn().
				Str("agent", msg.GetSource()).
				Str("error", messages.GetErrorDetail(msg)).
				Msg("Agent error, skipping to next participant")
			m.SelectAndPublish(ctx, msg)
			return
		default:
			stopMsg := messages.NewStopMessage(
				"manager",
				"team terminated due to error from "+msg.GetSource()+": "+messages.GetErrorDetail(msg),
			)
			m.ctx.Runtime.Publish(ctx, m.ctx.OutputTopic, stopMsg)
			return
		}
	}

	if messages.IsHandoffMessage(msg) {
		target := messages.GetHandoffTarget(msg)
		if idx := m.findParticipant(target); idx >= 0 {
			m.mu.Lock()
			m.currentIndex = idx + 1
			m.mu.Unlock()
			m.ctx.Runtime.Publish(ctx, m.ctx.ParticipantTopics[idx], msg)
			return
		}
	}

	m.SelectAndPublish(ctx, msg)
}

type SelectorGroupChat struct {
	*BaseGroupChat
	manager *SelectorManager
}

func NewSelectorGroupChat(name string, participants []agent.ChatAgent, maxTurns int, errorPolicy ErrorPolicy, llm agent.LLMProcessor, maxHistory int) (*SelectorGroupChat, error) {
	mgr := NewSelectorManager()
	if maxHistory > 0 {
		mgr.maxHistory = maxHistory
	}

	var term TerminationCondition
	if maxTurns > 0 {
		term = NewMaxTurnsTermination(maxTurns)
	}

	base, err := NewBaseGroupChat(BaseGroupChatConfig{
		Name:         name,
		Description:  "Selector group chat",
		Participants: participants,
		Manager:      mgr,
		Termination:  term,
		ErrorPolicy:  errorPolicy,
		LLM:          llm,
	})
	if err != nil {
		return nil, err
	}

	logger.L().Info().Str("group chat", name).Msg("successfully launch selector group chat")
	return &SelectorGroupChat{
		BaseGroupChat: base,
		manager:       mgr,
	}, nil
}

var _ Team = (*SelectorGroupChat)(nil)
