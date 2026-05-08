package team

import (
	"context"
	"sync"

	"brambleclaw/internal/agent"
	"brambleclaw/internal/messages"
)

// RoundRobinManager 轮询管理器，按顺序选择下一个参与者
// 轮询选择参与者 : 按照预设的顺序，依次选择团队中的 Agent （即 Participants ）来处理消息。默认情况下，
// RoundRobinManager 会按照预设的顺序（例如，Sales -> Support -> Technical -> Sales...）将消息分发给团队中的每个智能体。这通过 currentIndex 的递增和取模运算实现
// 处理任务和响应 : 接收来自管理主题 ( ManagerTopic ) 的任务消息和来自群组主题 ( GroupTopic ) 的响应消息，并将其转发给下一个选定的 Agent 。
// 处理 Handoff 消息 : 如果收到 Handoff 消息（表示将控制权移交给特定 Agent ）， RoundRobinManager 会查找目标 Agent ，并将其设置为下一个处理消息的 Agent ，从而打破轮询顺序，实现更灵活的协作。
type RoundRobinManager struct {
	ctx          GroupChatManagerContext
	currentIndex int
	mu           sync.Mutex
	cancel       context.CancelFunc
}

// NewRoundRobinManager 创建轮询管理器
func NewRoundRobinManager() *RoundRobinManager {
	return &RoundRobinManager{}
}

func (m *RoundRobinManager) Initialize(_ context.Context, context GroupChatManagerContext) error {
	m.ctx = context
	m.currentIndex = 0
	return nil
}

func (m *RoundRobinManager) Start(ctx context.Context) error {
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

func (m *RoundRobinManager) Pause(_ context.Context) error { return nil }

func (m *RoundRobinManager) Resume(_ context.Context) error { return nil }

func (m *RoundRobinManager) Reset(_ context.Context) error {
	m.mu.Lock()
	m.currentIndex = 0
	m.mu.Unlock()
	return nil
}

// Stop 停止管理器（非接口方法，供外部显式停止）
func (m *RoundRobinManager) Stop() {
	if m.cancel != nil {
		m.cancel()
	}
}

func (m *RoundRobinManager) handleTask(ctx context.Context, msg messages.ChatMessage) {
	m.ctx.Runtime.Publish(ctx, m.ctx.OutputTopic, msg)
	m.selectAndPublish(ctx, msg)
}

func (m *RoundRobinManager) handleResponse(ctx context.Context, msg messages.ChatMessage) {
	m.ctx.Runtime.Publish(ctx, m.ctx.OutputTopic, msg)

	if messages.IsStopMessage(msg) {
		return
	}

	if messages.IsHandoffMessage(msg) {
		target := messages.GetHandoffTarget(msg)
		if idx := m.findParticipant(target); idx >= 0 {
			m.mu.Lock()
			m.currentIndex = idx
			m.mu.Unlock()
			m.selectAndPublish(ctx, msg)
			return
		}
	}

	m.selectAndPublish(ctx, msg)
}

func (m *RoundRobinManager) selectAndPublish(ctx context.Context, msg messages.ChatMessage) {
	m.mu.Lock()
	idx := m.currentIndex
	m.currentIndex = (m.currentIndex + 1) % len(m.ctx.Participants)
	m.mu.Unlock()

	m.ctx.Runtime.Publish(ctx, m.ctx.ParticipantTopics[idx], msg)
}

func (m *RoundRobinManager) findParticipant(name string) int {
	for i, p := range m.ctx.Participants {
		if p.Name() == name {
			return i
		}
	}
	return -1
}

// RoundRobinGroupChat 轮询群组聊天
type RoundRobinGroupChat struct {
	*BaseGroupChat
	manager *RoundRobinManager
}

// NewRoundRobinGroupChat 创建轮询群组聊天
func NewRoundRobinGroupChat(name string, participants []agent.ChatAgent, maxTurns int) (*RoundRobinGroupChat, error) {
	mgr := NewRoundRobinManager()

	var term TerminationCondition
	if maxTurns > 0 {
		term = NewMaxTurnsTermination(maxTurns)
	}

	base, err := NewBaseGroupChat(BaseGroupChatConfig{
		Name:         name,
		Description:  "Round robin group chat",
		Participants: participants,
		Manager:      mgr,
		Termination:  term,
	})
	if err != nil {
		return nil, err
	}

	return &RoundRobinGroupChat{
		BaseGroupChat: base,
		manager:       mgr,
	}, nil
}

// 编译时检查
var _ Team = (*RoundRobinGroupChat)(nil)
