package team

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"

	"neoclaw/internal/agent"
	"neoclaw/internal/logger"
	"neoclaw/internal/messages"
	"neoclaw/internal/runtime"

	"github.com/google/uuid"
)

// GroupChatManagerContext 管理器上下文，包含运行时和拓扑信息
type GroupChatManagerContext struct {
	Runtime           messages.RuntimeProvider
	GroupTopic        string   // GroupChatManager 会订阅此 Topic： 当一个 Agent 在群聊中发言后，它的回复会发布到此 Topic ，然后被管理器捕获并进行下一步调度
	ManagerTopic      string   // GroupChatManager 会订阅此 Topic，以接收外部系统发送给群聊管理器的任务消息
	ParticipantTopics []string // 群聊中每一个Agent专属的消息主题
	OutputTopic       string   // 向外部报告群聊进展和结果
	Participants      []agent.ChatAgent
	ErrorPolicy       ErrorPolicy
	LLM               agent.LLMProcessor
}

// GroupChatManager 群聊管理器接口，控制消息流转和参与者选择
type GroupChatManager interface {
	Initialize(ctx context.Context, context GroupChatManagerContext) error
	Start(ctx context.Context) error
	Pause(ctx context.Context) error
	Resume(ctx context.Context) error
	Reset(ctx context.Context) error
	SelectAndPublish(ctx context.Context, msg messages.ChatMessage) error
}

// BaseGroupChatConfig 创建 BaseGroupChat 的配置
type BaseGroupChatConfig struct {
	Name         string
	Description  string
	Participants []agent.ChatAgent
	Manager      GroupChatManager
	Termination  TerminationCondition
	ErrorPolicy  ErrorPolicy
	Runtime      *runtime.AgentRuntime
	LLM          agent.LLMProcessor
}

// BaseGroupChat 基础群组聊天实现
type BaseGroupChat struct {
	base         *agent.BaseChatAgent
	participants []agent.ChatAgent
	manager      GroupChatManager
	termination  TerminationCondition
	errorPolicy  ErrorPolicy
	rt           *runtime.AgentRuntime
	llm          agent.LLMProcessor

	mu      sync.Mutex
	running atomic.Bool

	// topic IDs
	teamID            string
	groupTopic        string
	managerTopic      string
	participantTopics []string
	outputTopic       string

	// containers
	containers []*runtime.ChatAgentContainer
}

// NewBaseGroupChat 创建基础群组聊天
func NewBaseGroupChat(cfg BaseGroupChatConfig) (*BaseGroupChat, error) {
	if len(cfg.Participants) == 0 {
		return nil, errors.New("team: at least one participant required")
	}

	names := make(map[string]bool, len(cfg.Participants))
	for _, p := range cfg.Participants {
		if names[p.Name()] {
			return nil, fmt.Errorf("team: duplicate participant name: %s", p.Name())
		}
		names[p.Name()] = true
	}

	// 设置默认错误策略
	errorPolicy := cfg.ErrorPolicy
	if errorPolicy == "" {
		errorPolicy = ErrorPolicyTerminate
	}

	// 如果使用 terminate 策略，自动添加 StopMessageTermination
	termination := cfg.Termination
	if errorPolicy == ErrorPolicyTerminate {
		stopTerm := NewStopMessageTermination()
		if termination != nil {
			termination = NewAllTermination(termination, stopTerm)
		} else {
			termination = stopTerm
		}
	}

	teamID := uuid.NewString()
	rt := cfg.Runtime
	if rt == nil {
		rt = runtime.NewAgentRuntime()
	}

	participantTopics := make([]string, len(cfg.Participants))
	for i, p := range cfg.Participants {
		participantTopics[i] = fmt.Sprintf("%s_%s", p.Name(), teamID)
	}

	return &BaseGroupChat{
		base:              agent.NewBaseChatAgent(cfg.Name, cfg.Description),
		participants:      cfg.Participants,
		manager:           cfg.Manager,
		termination:       termination,
		errorPolicy:       errorPolicy,
		rt:                rt,
		llm:               cfg.LLM,
		teamID:            teamID,
		groupTopic:        fmt.Sprintf("group_topic_%s", teamID),
		managerTopic:      fmt.Sprintf("manager_topic_%s", teamID),
		participantTopics: participantTopics,
		outputTopic:       fmt.Sprintf("output_topic_%s", teamID),
	}, nil
}

// Run 执行任务，收集所有消息后返回
func (c *BaseGroupChat) Run(ctx context.Context, task messages.ChatMessage) (*TaskResult, error) {
	if !c.running.CompareAndSwap(false, true) {
		return nil, errors.New("team: group chat already running")
	}
	defer c.running.Store(false)

	if err := c.initialize(ctx); err != nil {
		return nil, err
	}
	defer c.cleanup()

	if err := c.manager.Start(ctx); err != nil {
		return nil, err
	}
	logger.L().Debug().Msg("group chat manager started")
	c.rt.Publish(ctx, c.managerTopic, task)

	sub := c.rt.Subscribe(ctx, c.outputTopic)
	result := &TaskResult{Messages: make([]messages.ChatMessage, 0)}

	// 填充错误信息的辅助函数
	populateError := func() {
		var errs []string
		for _, msg := range result.Messages {
			if messages.IsErrorMessage(msg) {
				errs = append(errs, msg.GetSource()+": "+messages.GetErrorDetail(msg))
			}
		}
		if len(errs) > 0 {
			result.Error = strings.Join(errs, "; ")
		}
	}

	for {
		select {
		case <-ctx.Done():
			populateError()
			return result, ctx.Err()
		case msg, ok := <-sub.Ch():
			if !ok {
				populateError()
				return result, nil
			}
			chatMsg, ok := msg.(messages.ChatMessage)
			if !ok {
				continue
			}
			result.Messages = append(result.Messages, chatMsg)
			logger.L().Debug().Msg("group chat message received")

			if c.termination != nil && c.termination.ShouldTerminate(chatMsg) {
				populateError()
				return result, nil
			}
		}
	}
}

// RunStream 流式执行任务，逐条推送 StreamItem
func (c *BaseGroupChat) RunStream(ctx context.Context, task messages.ChatMessage) (<-chan agent.StreamItem, error) {
	if !c.running.CompareAndSwap(false, true) {
		return nil, errors.New("team: group chat already running")
	}

	if err := c.initialize(ctx); err != nil {
		c.running.Store(false)
		return nil, err
	}

	if err := c.manager.Start(ctx); err != nil {
		c.cleanup()
		c.running.Store(false)
		return nil, err
	}

	c.rt.Publish(ctx, c.managerTopic, task)

	sub := c.rt.Subscribe(ctx, c.outputTopic)
	ch := make(chan agent.StreamItem, 64)

	go func() {
		defer close(ch)
		defer c.cleanup()
		defer c.running.Store(false)

		for {
			select {
			case <-ctx.Done():
				ch <- agent.StreamItem{Err: ctx.Err()}
				return
			case msg, ok := <-sub.Ch():
				if !ok {
					return
				}
				chatMsg, ok := msg.(messages.ChatMessage)
				if !ok {
					continue
				}

				item := agent.StreamItem{Message: chatMsg}

				shouldTerminate := c.termination != nil && c.termination.ShouldTerminate(chatMsg)
				if shouldTerminate {
					item.Response = &agent.Response{ChatMessage: chatMsg}
					ch <- item
					return
				}

				ch <- item
			}
		}
	}()

	return ch, nil
}

// ChatAgent 接口实现

func (c *BaseGroupChat) Name() string        { return c.base.Name() }
func (c *BaseGroupChat) Description() string { return c.base.Description() }

func (c *BaseGroupChat) ProducedMessageTypes() []messages.MessageType {
	seen := make(map[messages.MessageType]bool)
	var types []messages.MessageType
	for _, p := range c.participants {
		for _, t := range p.ProducedMessageTypes() {
			if !seen[t] {
				seen[t] = true
				types = append(types, t)
			}
		}
	}
	return types
}

func (c *BaseGroupChat) OnMessages(ctx context.Context, msgs []messages.ChatMessage) (*agent.Response, error) {
	if len(msgs) == 0 {
		return nil, errors.New("team: no messages to process")
	}
	result, err := c.Run(ctx, msgs[0])
	if err != nil {
		return nil, err
	}
	if len(result.Messages) == 0 {
		return &agent.Response{}, nil
	}
	lastMsg := result.Messages[len(result.Messages)-1]
	return &agent.Response{ChatMessage: lastMsg}, nil
}

func (c *BaseGroupChat) OnMessagesStream(ctx context.Context, msgs []messages.ChatMessage) (<-chan agent.StreamItem, error) {
	if len(msgs) == 0 {
		return nil, errors.New("team: no messages to process")
	}
	return c.RunStream(ctx, msgs[0])
}

func (c *BaseGroupChat) OnReset(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.cleanup()
	if c.termination != nil {
		c.termination.Reset()
	}
	if err := c.manager.Reset(ctx); err != nil {
		return err
	}
	return c.base.OnReset(ctx)
}

func (c *BaseGroupChat) OnPause(ctx context.Context) error {
	return c.base.OnPause(ctx)
}

func (c *BaseGroupChat) OnResume(ctx context.Context) error {
	return c.base.OnResume(ctx)
}

func (c *BaseGroupChat) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.cleanup()
	return c.base.Close()
}

func (c *BaseGroupChat) SaveState() (map[string]any, error) {
	return c.base.SaveState()
}

func (c *BaseGroupChat) LoadState(state map[string]any) error {
	return c.base.LoadState(state)
}

// 内部方法

func (c *BaseGroupChat) initialize(ctx context.Context) error {
	mgrCtx := GroupChatManagerContext{
		Runtime:           c.rt,
		GroupTopic:        c.groupTopic,
		ManagerTopic:      c.managerTopic,
		ParticipantTopics: c.participantTopics,
		OutputTopic:       c.outputTopic,
		Participants:      c.participants,
		ErrorPolicy:       c.errorPolicy,
		LLM:               c.llm,
	}
	if err := c.manager.Initialize(ctx, mgrCtx); err != nil {
		return err
	}

	c.containers = make([]*runtime.ChatAgentContainer, len(c.participants))
	for i, p := range c.participants {
		container := runtime.NewChatAgentContainer(
			p,
			c.rt,
			runtime.TopicID(c.participantTopics[i]),
			runtime.TopicID(c.groupTopic),
		)
		if err := container.Start(ctx); err != nil {
			return err
		}
		c.containers[i] = container
	}

	return nil
}

func (c *BaseGroupChat) cleanup() {
	for _, container := range c.containers {
		container.Stop()
	}
	c.containers = nil

	c.rt.RemoveTopic(runtime.TopicID(c.groupTopic))
	c.rt.RemoveTopic(runtime.TopicID(c.managerTopic))
	c.rt.RemoveTopic(runtime.TopicID(c.outputTopic))
	for _, pt := range c.participantTopics {
		c.rt.RemoveTopic(runtime.TopicID(pt))
	}

	logger.L().Debug().Str("team", c.Name()).Msg("Group chat cleaned up")
}

// 编译时检查
var _ Team = (*BaseGroupChat)(nil)
