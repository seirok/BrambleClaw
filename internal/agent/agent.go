package agent

import (
	util "brambleclaw/internal"
	"brambleclaw/internal/bus"
	"brambleclaw/internal/config"
	"brambleclaw/internal/hook"
	"brambleclaw/internal/interfaces"
	"brambleclaw/internal/logger"
	"brambleclaw/internal/messages"
	"brambleclaw/internal/sandbox"
	"brambleclaw/internal/session"
	"brambleclaw/internal/skill"
	"brambleclaw/internal/tools"
	"brambleclaw/internal/tools/mcp"
	"context"
	"strings"
)

// SkillInfo 是 /help 命令展示的技能信息（避免暴露 skill 包类型）
type SkillInfo struct {
	Name        string
	Description string
}

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
	skillManager   interface{} // *skill.SkillManager
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

func WithWorkspace(workspace string) Option {
	return func(a *Agent) {
		a.workspace = workspace
	}
}

// WithSkillManager 设置技能管理器
func WithSkillManager(sm *skill.SkillManager) Option {
	return func(a *Agent) {
		a.skillManager = sm
		// Also set on ContextBuilder
		if a.builder != nil {
			a.builder.SetSkillManager(sm)
		}
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

// ListUserInvocableSkills 返回用户可调用的技能列表，给 /help 命令用
func (a *Agent) ListUserInvocableSkills() interface{} {
	if a.skillManager == nil {
		return nil
	}
	sm, ok := a.skillManager.(*skill.SkillManager)
	if !ok {
		return nil
	}
	metas := sm.ListMeta(context.Background())
	result := make([]struct{ Name, Description string }, 0, len(metas))
	for _, m := range metas {
		if m.UserInvocable {
			result = append(result, struct{ Name, Description string }{
				Name:        m.Name,
				Description: m.Description,
			})
		}
	}
	return result
}

// ListAllSkills 返回所有技能的详细信息（不做 UserInvocable 过滤），给 /skills 命令用
func (a *Agent) ListAllSkills() interface{} {
	if a.skillManager == nil {
		return nil
	}
	sm, ok := a.skillManager.(*skill.SkillManager)
	if !ok {
		return nil
	}
	metas := sm.ListMeta(context.Background())
	type skillArg struct {
		Name        string
		Description string
		Required    bool
		Default     string
	}
	result := make([]struct {
		Name               string
		Description        string
		UserInvocable      bool
		DisableModelInvoke bool
		Scope              string
		Arguments          []skillArg
	}, 0, len(metas))
	for _, m := range metas {
		args := make([]skillArg, 0, len(m.Arguments))
		for _, arg := range m.Arguments {
			args = append(args, skillArg{
				Name:        arg.Name,
				Description: arg.Description,
				Required:    arg.Required,
				Default:     arg.Default,
			})
		}
		var scopeStr string
		switch m.Scope {
		case skill.ScopePlugin:
			scopeStr = "plugin"
		case skill.ScopePersonal:
			scopeStr = "personal"
		case skill.ScopeProject:
			scopeStr = "project"
		case skill.ScopeEnterprise:
			scopeStr = "enterprise"
		default:
			scopeStr = "unknown"
		}
		result = append(result, struct {
			Name               string
			Description        string
			UserInvocable      bool
			DisableModelInvoke bool
			Scope              string
			Arguments          []skillArg
		}{
			Name:               m.Name,
			Description:        m.Description,
			UserInvocable:      m.UserInvocable,
			DisableModelInvoke: m.DisableModelInvoke,
			Scope:              scopeStr,
			Arguments:          args,
		})
	}
	return result
}

// CurrentModel 返回当前使用的 LLM 模型名
func (a *Agent) CurrentModel() string {
	if a.orche == nil {
		return ""
	}
	return a.orche.LLM().Model()
}

// SwitchModel 切换到指定的 LLM 模型
func (a *Agent) SwitchModel(model string) error {
	if a.orche == nil {
		return nil
	}
	llm := a.orche.LLM()
	if llmClient, ok := llm.(*LLMClient); ok {
		llmClient.SetModel(model)
		return nil
	}
	return nil
}

// ResetSession 重置当前会话，清空消息但不创建新会话
func (a *Agent) ResetSession(sessionKey string) error {
	if _, err := a.SessionMgr().ClearSessionWithMeta(sessionKey); err != nil {
		return err
	}
	if a.builder != nil {
		a.builder.ResetCompressor()
	}
	return a.OnReset(context.Background())
}

// UndoLastRound 撤销上一轮对话（移除最后一条 user + 最后一条 assistant 消息）
func (a *Agent) UndoLastRound(sessionKey string) (int, error) {
	sess, err := a.SessionMgr().Get(context.Background(), sessionKey)
	if err != nil {
		return 0, err
	}
	if len(sess.Messages) <= 1 {
		return 0, nil
	}
	removed := 0
	// 从末尾移除 assistant
	for i := len(sess.Messages) - 1; i >= 0; i-- {
		if am, ok := sess.Messages[i].(*AgentMessage); ok && am.Role == RoleAssistant {
			sess.Messages = append(sess.Messages[:i], sess.Messages[i+1:]...)
			removed++
			break
		}
	}
	// 从末尾移除 user
	for i := len(sess.Messages) - 1; i >= 0; i-- {
		if am, ok := sess.Messages[i].(*AgentMessage); ok && am.Role == RoleUser {
			sess.Messages = append(sess.Messages[:i], sess.Messages[i+1:]...)
			removed++
			break
		}
	}
	// 调整 Summarized 指针
	if sess.Summarized > len(sess.Messages) {
		sess.Summarized = len(sess.Messages)
		if sess.Summarized < 1 {
			sess.Summarized = 1
		}
	}
	if removed > 0 {
		sess.Modified = true
	}
	return removed, nil
}

// ForceCompactSession 手动触发上下文压缩，忽略阈值
func (a *Agent) ForceCompactSession(ctx context.Context, sessionKey string) (int, error) {
	sess, err := a.SessionMgr().Get(ctx, sessionKey)
	if err != nil {
		return 0, err
	}
	if a.builder == nil {
		return 0, nil
	}
	// 构建 DynamicInfo
	_, channel, chatID, _ := util.ParseSessionKey(sessionKey)
	info := &DynamicInfo{
		channel: channel,
		chatID:  chatID,
	}
	return a.builder.ForceCompact(ctx, sess, info)
}

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
	agentMsgs := make([]*AgentMessage, 0, len(msgs))
	for _, msg := range msgs {
		agentMsgs = append(agentMsgs, NewAgentMessage(msg.GetSource(), RoleUser, msg.ToModelText()))
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
			sess.AddMessage(NewAgentMessage(a.name, RoleSystem, fullSystemPrompt))
		}
	}

	// 添加用户消息到 session
	for _, am := range agentMsgs {
		sess.AddMessage(am)
	}

	historyMsg := sess.LoadHistory()

	// 使用 Orchestrator 处理（注入 session key 到 context）
	toolCtx := sandbox.ContextWithSessionKey(ctx, sessKey)
	resp, err := a.Orchestrator().Run(toolCtx, historyMsg)
	if err != nil {
		return nil, err
	}

	replyContent := ""
	if len(resp.Choices) > 0 {
		replyContent = resp.Choices[0].Message.Content
	}

	// 添加回复到 session
	replyMsg := NewAgentMessage(a.name, RoleAssistant, replyContent)
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
		// Type 2: route program error to TUI
		errMsg := messages.NewAgentErrorMessage(a.name, err.Error()).WithIsProgramError(true)
		errOutData := messages.ToOutBoundData(errMsg, msg.ChatID, msg.InChannel, msg.ID)
		errOutbound := &bus.OutBoundMessage{
			ChatID:     errOutData.ChatID,
			OutChannel: errOutData.Channel,
			Content:    errOutData.Content,
			MsgType:    errOutData.MsgType,
			ReplyTo:    errOutData.ReplyTo,
			TimeStamp:  errOutData.TimeStamp,
		}
		if pubErr := a.Bus().PublishOutBoundMessage(ctx, errOutbound); pubErr != nil {
			logger.L().Error().Err(pubErr).Str("channel", errOutbound.OutChannel).Msg("Error publishing error response")
		}
		return
	}

	// 将 ChatMessage 响应转换为 OutBoundMessage
	outData := messages.ToOutBoundData(response.ChatMessage, msg.ChatID, msg.InChannel, msg.ID)
	outbound := &bus.OutBoundMessage{
		ChatID:     outData.ChatID,
		OutChannel: outData.Channel,
		Content:    outData.Content,
		MsgType:    outData.MsgType,
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
	_, count, err := a.SessionMgr().RenewSession(sessionKey)
	if err != nil {
		logger.L().Error().Err(err).Msg("Failed to renew session")
		return 0, err
	}
	logger.L().Info().Int("count", count).Msg("Session renewed with new key")
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
