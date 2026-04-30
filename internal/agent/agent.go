package agent

import (
	util "brambleclaw/internal"
	"brambleclaw/internal/bus"
	"brambleclaw/internal/config"
	"brambleclaw/internal/hook"
	"brambleclaw/internal/interfaces"
	"brambleclaw/internal/logger"
	"brambleclaw/internal/session"
	"brambleclaw/internal/tools"
	"brambleclaw/internal/tools/mcp"
	"context"
	"strings"
	"time"
)

type Agent struct {
	name           string
	workspace      string
	bus            *bus.MessageBus
	orche          *Orchestrator
	tools          interfaces.Registry[tools.Tool]
	sessionManager *session.PersistentSessionManager
	mcp            *mcp.Manager
	builder        *ContextBuilder
	commands       interfaces.Registry[interfaces.Command]
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

// WithWorkspace 设置工作区
func WithWorkspace(workspace string) Option {
	return func(a *Agent) {
		a.workspace = workspace
	}
}

func (a *Agent) Bus() *bus.MessageBus { return a.bus }

func (a *Agent) SessionMgr() *session.PersistentSessionManager { return a.sessionManager }

func (a *Agent) Orchestrator() *Orchestrator { return a.orche }

func (a *Agent) ContextBuilder() *ContextBuilder { return a.builder }

func (a *Agent) Commands() interfaces.Registry[interfaces.Command] { return a.commands }

func (a *Agent) Workspace() string { return a.workspace }

func (a *Agent) Name() string { return a.name }

// handleMessage 处理入站消息
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

	// 构建系统提示
	sessKey := util.BuildSessionKey(a.name, msg.InChannel, msg.ChatID)

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

	sess, isNewSession := a.SessionMgr().GetOrCreate(sessKey)

	dynamicInfo := &DynamicInfo{
		channel:  msg.InChannel,
		chatID:   msg.ChatID,
		senderID: msg.SenderID,
	}

	if isNewSession || len(sess.Messages) == 0 {
		fullSystemPrompt, err := a.ContextBuilder().BuildFullSystemPrompt(dynamicInfo)
		if err != nil {
			logger.L().Error().Err(err).Msg("Failed to build full system prompt")
			fullSystemPrompt = ""
		} else {
			sess.AddMessage(AgentMessage{
				Role:      RoleSystem,
				Content:   []ContentBlock{TextContent{fullSystemPrompt}},
				Timestamp: time.Now().UnixMilli(),
			})
		}
	}
	// 添加当前消息
	agentMsg := AgentMessage{
		Role:      RoleUser,
		Content:   []ContentBlock{TextContent{Text: msg.Content}},
		Timestamp: time.Now().UnixMilli(),
	}
	sess.AddMessage(agentMsg)

	// 加载历史消息
	historyMsg := sess.LoadHistory()

	// 使用Orchestrator处理消息
	resp, err := a.Orchestrator().Run(ctx, historyMsg)

	if err != nil {
		logger.L().Error().Err(err).Msg("Failed to complete communication with LLM")
		return
	}

	replyMsg := AgentMessage{
		Role:      RoleAssistant,
		Content:   []ContentBlock{TextContent{Text: resp.Choices[0].Message.Content}},
		Timestamp: time.Now().UnixMilli(),
	}
	// 添加回复到会话
	sess.AddMessage(replyMsg)

	// 更新会话
	currentTokenUsed := resp.Usage.CompletionTokens + resp.Usage.PromptTokens
	a.SessionMgr().Update(sess, currentTokenUsed)

	go func() {
		dynamicInfo.usage = currentTokenUsed
		if err = a.ContextBuilder().Compact(ctx, sess, dynamicInfo); err != nil {
			logger.L().Error().Err(err).Msg("Failed to compact session")
		}
	}()

	// 发布响应
	outbound := &bus.OutBoundMessage{
		OutChannel: msg.InChannel,
		ChatID:     msg.ChatID,
		Content:    resp.Choices[0].Message.Content,
		ReplyTo:    msg.ID,
		TimeStamp:  time.Now(),
	}
	// 触发响应前钩子
	if processedOutbound, err := hook.Emit(ctx, "hook.point.message.pre-response", outbound); err != nil {
		logger.L().Error().Err(err).Msg("Pre-response hook failed")
		return
	} else if processedOutbound != nil {
		if m, ok := processedOutbound.(*bus.OutBoundMessage); ok {
			outbound = m
		}
	}

	if err := a.Bus().PublishOutBoundMessage(ctx, outbound); err != nil {
		logger.L().Error().Err(err).Msg("Error publishing response")
	}

	// 触发消息处理后钩子
	hook.Emit(ctx, "hook.point.message.post-process", msg)
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
	hook.Emit(ctx, "hook.point.agent.start", a)

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
	hook.Emit(ctx, "hook.point.agent.stop", a)

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
