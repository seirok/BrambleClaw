package agent

import (
	util "brambleclaw/internal"
	"brambleclaw/internal/bus"
	"brambleclaw/internal/config"
	"brambleclaw/internal/hook"
	"brambleclaw/internal/interfaces"
	"brambleclaw/internal/logger"
	"brambleclaw/internal/messages"
	"brambleclaw/internal/session"
	"brambleclaw/internal/tools"
	"brambleclaw/internal/tools/mcp"
	"context"
	"strings"
	"time"
)

type Agent struct {
	name           string
	description    string
	workspace      string
	bus            *bus.MessageBus
	orche          *Orchestrator
	tools          interfaces.Registry[tools.Tool]
	sessionManager *session.PersistentSessionManager
	mcp            *mcp.Manager
	builder        *ContextBuilder
	commands       interfaces.Registry[interfaces.Command]
	base           *BaseChatAgent
	runtime        messages.RuntimeProvider
}

// Option 是用于配置 Agent 的函数类型
type Option func(*Agent)

// NewAgent 创建一个新的 Agent 实例，使用 Functional Options 模式
func NewAgent(name string, opts ...Option) *Agent {
	agent := &Agent{name: name}

	// 应用所有选项
	for _, opt := range opts {
		opt(agent)
	}

	// 初始化 BaseChatAgent
	agent.base = NewBaseChatAgent(agent.name, agent.description)

	// 触发 Agent 创建钩子
	hook.Emit(context.Background(), "hook.point.agent.create", agent)

	return agent
}

// WithBus 设置消息总线
func WithBus(bus *bus.MessageBus) Option {
	return func(a *Agent) {
		a.bus = bus
	}
}

// WithOrchestrator 设置编排器
func WithOrchestrator(orche *Orchestrator) Option {
	return func(a *Agent) {
		a.orche = orche
	}
}

// WithTools 设置工具注册表
func WithTools(t interfaces.Registry[tools.Tool]) Option {
	return func(a *Agent) {
		a.tools = t
	}
}

// WithSessionManager 设置会话管理器
func WithSessionManager(sm *session.PersistentSessionManager) Option {
	return func(a *Agent) {
		a.sessionManager = sm
	}
}

// WithMCP 设置 MCP 管理器
func WithMCP(m *mcp.Manager) Option {
	return func(a *Agent) {
		a.mcp = m
	}
}

// WithContextBuilder 设置上下文构建器
func WithContextBuilder(cb *ContextBuilder) Option {
	return func(a *Agent) {
		a.builder = cb
	}
}

// WithCommands 设置命令注册表
func WithCommands(cmds interfaces.Registry[interfaces.Command]) Option {
	return func(a *Agent) {
		a.commands = cmds
	}
}

// WithDescription 设置描述
func WithDescription(desc string) Option {
	return func(a *Agent) {
		a.description = desc
	}
}

// WithRuntime 设置运行时
func WithRuntime(r messages.RuntimeProvider) Option {
	return func(a *Agent) {
		a.runtime = r
	}
}

func (a *Agent) Bus() *bus.MessageBus { return a.bus }

func (a *Agent) SessionMgr() *session.PersistentSessionManager { return a.sessionManager }

func (a *Agent) Orchestrator() *Orchestrator { return a.orche }

func (a *Agent) ContextBuilder() *ContextBuilder { return a.builder }

func (a *Agent) Commands() interfaces.Registry[interfaces.Command] { return a.commands }

func (a *Agent) Tools() interfaces.Registry[tools.Tool] { return a.tools }

func (a *Agent) Workspace() string { return a.workspace }

func (a *Agent) Name() string { return a.name }

func (a *Agent) Runtime() messages.RuntimeProvider { return a.runtime }

// Description 返回 Agent 描述（ChatAgent 接口）
func (a *Agent) Description() string { return a.description }

// ProducedMessageTypes 返回 Agent 可产生的消息类型（ChatAgent 接口）
func (a *Agent) ProducedMessageTypes() []messages.MessageType {
	return []messages.MessageType{
		messages.MessageTypeText,
		messages.MessageTypeToolCallSum,
		messages.MessageTypeHandoff,
	}
}

// OnMessages 处理 ChatMessage 列表，返回 Agent 响应（ChatAgent 接口核心方法）
func (a *Agent) OnMessages(ctx context.Context, msgs []messages.ChatMessage) (*Response, error) {
	// 将 ChatMessage 转换为 AgentMessage 并收集到 session
	agentMsgs := make([]AgentMessage, 0, len(msgs))
	for _, msg := range msgs {
		agentMsgs = append(agentMsgs, AgentMessage{
			Role:      RoleUser,
			Content:   []ContentBlock{TextContent{Text: msg.ToModelText()}},
			Timestamp: time.Now().UnixMilli(),
		})
	}

	// 如果有 system prompt 需要，从第一条消息的 metadata 获取上下文信息
	channel := ""
	chatID := ""
	senderID := ""
	if len(msgs) > 0 {
		if meta := msgs[0].GetMetadata(); meta != nil {
			channel = meta["channel"]
			chatID = meta["chat_id"]
			senderID = msgs[0].GetSource()
		}
	}

	sessKey := util.BuildSessionKey(a.name, channel, chatID)
	sess, isNewSession := a.SessionMgr().GetOrCreate(sessKey)

	dynamicInfo := &DynamicInfo{
		channel:  channel,
		chatID:   chatID,
		senderID: senderID,
	}

	if isNewSession || len(sess.Messages) == 0 {
		fullSystemPrompt, err := a.ContextBuilder().Build(dynamicInfo)
		if err != nil {
			logger.L().Error().Err(err).Str("agent", a.name).Str("channel", channel).Msg("Failed to build full system prompt")
			fullSystemPrompt = ""
		} else {
			sess.AddMessage(AgentMessage{
				Role:      RoleSystem,
				Content:   []ContentBlock{TextContent{fullSystemPrompt}},
				Timestamp: time.Now().UnixMilli(),
			})
		}
	}

	// 添加用户消息到 session
	for _, am := range agentMsgs {
		sess.AddMessage(am)
	}

	historyMsg := sess.LoadHistory()

	// 使用 Orchestrator 处理
	resp, err := a.Orchestrator().Run(ctx, historyMsg)
	if err != nil {
		return nil, err
	}

	replyContent := ""
	if len(resp.Choices) > 0 {
		replyContent = resp.Choices[0].Message.Content
	}

	// 添加回复到 session
	replyMsg := AgentMessage{
		Role:      RoleAssistant,
		Content:   []ContentBlock{TextContent{Text: replyContent}},
		Timestamp: time.Now().UnixMilli(),
	}
	sess.AddMessage(replyMsg)

	// 更新 session
	currentTokenUsed := resp.Usage.CompletionTokens + resp.Usage.PromptTokens
	a.SessionMgr().Update(sess, currentTokenUsed)

	go func() {
		dynamicInfo.usage = currentTokenUsed
		if err = a.ContextBuilder().Compact(ctx, sess, dynamicInfo); err != nil {
			logger.L().Error().Err(err).Str("session_key", sessKey).Msg("Failed to compact session")
		}
	}()

	// 构建 ChatMessage 响应
	chatResp := messages.NewTextMessage(a.name, replyContent)
	return &Response{ChatMessage: chatResp}, nil
}

// OnMessagesStream 流式处理消息（ChatAgent 接口）
func (a *Agent) OnMessagesStream(ctx context.Context, msgs []messages.ChatMessage) (<-chan StreamItem, error) {
	return streamFromOnMessages(ctx, msgs, a.OnMessages), nil
}

// OnReset 重置 Agent 状态（ChatAgent 接口）
func (a *Agent) OnReset(ctx context.Context) error {
	return a.base.OnReset(ctx)
}

// OnPause 暂停 Agent（ChatAgent 接口）
func (a *Agent) OnPause(ctx context.Context) error {
	return a.base.OnPause(ctx)
}

// OnResume 恢复 Agent（ChatAgent 接口）
func (a *Agent) OnResume(ctx context.Context) error {
	return a.base.OnResume(ctx)
}

// Close 关闭 Agent（ChatAgent 接口）
func (a *Agent) Close() error {
	return a.Stop(context.Background())
}

// SaveState 保存 Agent 状态（ChatAgent 接口）
func (a *Agent) SaveState() (map[string]any, error) {
	return a.base.SaveState()
}

// LoadState 加载 Agent 状态（ChatAgent 接口）
func (a *Agent) LoadState(state map[string]any) error {
	return a.base.LoadState(state)
}

// IsPaused 检查是否暂停
func (a *Agent) IsPaused() bool {
	return a.base.IsPaused()
}

// 编译时检查：确保 Agent 实现了 ChatAgent 接口
var _ ChatAgent = (*Agent)(nil)

// HandleMessage 处理入站消息（外部入口，包含 Hook/Command 等横切关注点）
func (a *Agent) HandleMessage(ctx context.Context, msg *bus.InBoundMessage) {
	// 触发消息处理前钩子
	if processedMsg, err := hook.Emit(ctx, "hook.point.message.pre-process", msg); err != nil {
		logger.L().Error().Err(err).Msg("Message pre-processing hook failed")
		return
	} else if processedMsg != nil {
		if m, ok := processedMsg.(*bus.InBoundMessage); ok {
			msg = m
		}
	}

	content := strings.TrimSpace(msg.Content)

	// 检查是否是指令 (以 / 开头)
	if strings.HasPrefix(content, "/") {
		parts := strings.Fields(content[1:]) // 去掉 / 并按空格切分
		if len(parts) > 0 {
			cmdName := parts[0]
			args := parts[1:]

			if cmd, err := a.Commands().Get(ctx, cmdName); err == nil {
				if err := cmd.Execute(ctx, a, msg, args); err != nil {
					logger.L().Error().Err(err).Str("cmd", cmdName).Msg("Command execution failed")
				}
				return // 指令处理完直接返回，不走 LLM
			}
		}
	}

	// 转换为 ChatMessage 并委托给 OnMessages（内层核心）
	inBoundData := messages.InBoundData{
		ID:        msg.ID,
		SenderID:  msg.SenderID,
		ChatID:    msg.ChatID,
		InChannel: msg.InChannel,
		Content:   msg.Content,
	}
	chatMsg := messages.FromInBoundData(inBoundData)

	response, err := a.OnMessages(ctx, []messages.ChatMessage{chatMsg})
	if err != nil {
		logger.L().Error().Err(err).Msg("Failed to process message via OnMessages")
		return
	}

	// 将 ChatMessage 响应转换为 OutBoundMessage
	outData := messages.ToOutBoundData(response.ChatMessage, msg.ChatID, msg.InChannel, msg.ID)
	outbound := &bus.OutBoundMessage{
		ChatID:     outData.ChatID,
		OutChannel: outData.Channel,
		Content:    outData.Content,
		ReplyTo:    outData.ReplyTo,
		TimeStamp:  outData.TimeStamp,
	}

	// 触发响应前钩子
	if processedOutbound, err := hook.Emit(ctx, "hook.point.message.pre-response", outbound); err != nil {
		logger.L().Error().Err(err).Str("channel", msg.InChannel).Msg("Pre-response hook failed")
		return
	} else if processedOutbound != nil {
		if m, ok := processedOutbound.(*bus.OutBoundMessage); ok {
			outbound = m
		}
	}

	if err := a.Bus().PublishOutBoundMessage(ctx, outbound); err != nil {
		logger.L().Error().Err(err).Str("channel", outbound.OutChannel).Str("chat_id", outbound.ChatID).Msg("Error publishing response")
	}

	// 触发消息处理后钩子
	if _, err := hook.Emit(ctx, "hook.point.message.post-process", msg); err != nil {
		logger.L().Warn().Err(err).Str("channel", msg.InChannel).Msg("Post-process hook failed")
	}
}

func (a *Agent) ClearSession(sessionKey string) (int, error) {
	count, err := a.SessionMgr().ClearSessionWithMeta(sessionKey)
	if err != nil {
		logger.L().Error().Err(err).Msg("Failed to clear session")
		return 0, err
	}
	logger.L().Info().Int("count", count).Msg("Session cleared")
	return count, nil
}

func (a *Agent) Start(ctx context.Context) error {
	// 触发 Agent 启动前钩子
	if _, err := hook.Emit(ctx, "hook.point.agent.pre-start", a); err != nil {
		return err
	}

	cfg := config.Get()
	err := a.SessionMgr().Initialize(context.Background(), &cfg.Session)
	if err != nil {
		return err
	}

	err = a.SessionMgr().StartAll(ctx)
	if err != nil {
		return err
	}

	// 触发 Agent 启动后钩子
	if _, err := hook.Emit(ctx, "hook.point.agent.start", a); err != nil {
		logger.L().Warn().Err(err).Msg("Agent start hook failed")
	}

	return nil
}

// Stop 停止Agent
func (a *Agent) Stop(ctx context.Context) error {
	// 触发 Agent 停止前钩子
	if _, err := hook.Emit(ctx, "hook.point.agent.pre-stop", a); err != nil {
		logger.L().Error().Err(err).Msg("Failed to execute pre-stop hook")
	}

	// 停止 session manager
	err := a.sessionManager.StopAll(ctx)
	if err != nil {
		return err
	}

	// 关闭 MCP 管理器，释放资源
	a.mcp.Stop()

	// 触发 Agent 停止后钩子
	if _, err := hook.Emit(ctx, "hook.point.agent.stop", a); err != nil {
		logger.L().Warn().Err(err).Msg("Agent stop hook failed")
	}

	return nil
}

// GetOrCreateSession 获取或创建会话
// 这是提供给 Gateway 的公共方法
func (a *Agent) GetOrCreateSession(key string) *session.Session {
	sess, _ := a.SessionMgr().GetOrCreate(key)
	return sess
}

// GetSession 获取会话（如果不存在则返回 nil）
func (a *Agent) GetSession(key string) (*session.Session, error) {
	return a.SessionMgr().Get(context.Background(), key)
}
