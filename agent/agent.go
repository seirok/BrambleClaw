package agent

import (
	"brambleclaw/bus"
	"brambleclaw/config"
	util "brambleclaw/internal"
	"brambleclaw/logger"
	"brambleclaw/sandbox"
	"brambleclaw/tools"
	"brambleclaw/tools/mcp"
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

// AgentOption 配置选项函数类型
type AgentOption func(*agentOptions)

// agentOptions 代理配置选项
type agentOptions struct {
	sandboxCfg *config.SandboxConfig
	compactCfg *config.CompactConfig
}

// WithSandboxConfig 设置沙箱配置选项
func WithSandboxConfig(cfg *config.SandboxConfig) AgentOption {
	return func(opts *agentOptions) {
		opts.sandboxCfg = cfg
	}
}

// WithCompactConfig 设置压缩配置选项
func WithCompactConfig(cfg *config.CompactConfig) AgentOption {
	return func(opts *agentOptions) {
		opts.compactCfg = cfg
	}
}

// Agent Agent核心
type Agent struct {
	config         *config.AgentConfig
	sessionMgr     *PersistentSessionManager
	bus            *bus.MessageBus
	toolRegistry   *tools.ToolRegistry
	orchestrator   *Orchestrator
	mcpManager     *mcp.Manager
	contextBuilder *ContextBuilder
	workspace      string
	sandbox        *sandbox.Sandbox
}

// NewAgent 创建Agent
func NewAgent(cfg *config.AgentConfig, bus *bus.MessageBus, mcpManager *mcp.Manager, builder *ContextBuilder, opts ...AgentOption) *Agent {
	// 默认配置选项
	options := &agentOptions{}

	// 应用所有选项函数
	for _, opt := range opts {
		opt(options)
	}

	// 验证必需配置
	if options.sandboxCfg == nil {
		logger.L().Warn().Msg("sandbox config not provided, using default settings")
		options.sandboxCfg = &config.SandboxConfig{}
	}

	if options.compactCfg == nil {
		logger.L().Warn().Msg("compact config not provided, using default settings")
		options.compactCfg = &config.CompactConfig{}
	}

	llmClient := NewLLMClient(cfg.LLM)
	toolRegistry := tools.NewToolRegistry()
	orchestrator := NewOrchestrator(llmClient, toolRegistry)

	// 初始化 SummaryCompressor 并注入到 ContextBuilder
	compressor := NewSummaryCompressor(*options.compactCfg, llmClient)
	builder.summaryCompressor = compressor

	var sessionMgr *PersistentSessionManager
	sessionMgr = NewPersistentSessionManager(cfg.Workspace)

	// 使用配置的命令白名单，如果为空则使用默认
	allowedCommands := options.sandboxCfg.AllowedCommands
	if len(allowedCommands) == 0 {
		allowedCommands = []string{
			"echo", "cat", "type", "dir", "ls", "pwd", "cd",
			"mkdir", "rmdir", "rm", "cp", "mv", "copy", "move",
			"grep", "find", "head", "tail", "more", "less",
			"git", "go", "python", "python3", "node", "npm",
			"powershell", "cmd", "bash",
		}
	}

	timeout := time.Duration(options.sandboxCfg.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	maxOutputSize := options.sandboxCfg.MaxOutputSize
	if maxOutputSize <= 0 {
		maxOutputSize = 1024 * 1024 // 1MB
	}

	// 创建沙箱配置
	sandboxConfig := &sandbox.SandboxConfig{
		Enabled:          options.sandboxCfg.Enabled,
		Workspace:        cfg.Workspace,
		AllowReadOutside: false,
		FileSystem: sandbox.FileSystemConfig{
			MaxFileSize: 100 * 1024 * 1024, // 100MB
		},
		Execution: sandbox.ExecutionConfig{
			Timeout:         timeout,
			AllowedCommands: allowedCommands,
			MaxOutputSize:   maxOutputSize,
		},
	}

	// 创建沙箱（审计日志暂时为nil）
	sbx, err := sandbox.NewSandbox(sandboxConfig, nil)
	if err != nil {
		logger.L().Error().Err(err).Msg("创建沙箱失败，将使用无沙箱模式")
		sbx = nil
	}

	agent := &Agent{
		config:       cfg,
		sessionMgr:   sessionMgr,
		bus:          bus,
		toolRegistry: toolRegistry,
		orchestrator: orchestrator,
		mcpManager:   mcpManager,
		workspace:    cfg.Workspace,
		sandbox:      sbx,
	}

	builder.agent = agent
	agent.contextBuilder = builder

	return agent
}

func (a *Agent) RegisterTool(tool tools.Tool) {
	a.toolRegistry.Register(tool)
}

// handleMessage 处理入站消息
func (a *Agent) HandleMessage(ctx context.Context, msg *bus.InBoundMessage) {
	// 构建系统提示
	sessKey := util.BuildSessionKey(a.config.Name, msg.InChannel, msg.ChatID)

	// 处理 /clear 命令
	if strings.TrimSpace(msg.Content) == "/clear" {
		count, err := a.sessionMgr.ClearSession(sessKey)
		if err != nil {
			// 发送错误响应
			outbound := &bus.OutBoundMessage{
				OutChannel: msg.InChannel,
				ChatID:     msg.ChatID,
				Content:    fmt.Sprintf("Session clear failed: %v", err),
				ReplyTo:    msg.ID,
				TimeStamp:  time.Now(),
			}
			if err := a.bus.PublishOutBoundMessage(ctx, outbound); err != nil {
				logger.L().Error().Err(err).Msg("Error publishing clear error response")
			}
			return
		}

		// 发送成功响应
		outbound := &bus.OutBoundMessage{
			OutChannel: msg.InChannel,
			ChatID:     msg.ChatID,
			Content:    fmt.Sprintf("Session cleared. %d message(s) removed.", count),
			ReplyTo:    msg.ID,
			TimeStamp:  time.Now(),
		}
		if err := a.bus.PublishOutBoundMessage(ctx, outbound); err != nil {
			logger.L().Error().Err(err).Msg("Error publishing clear success response")
		}
		return
	}

	sess, isNewSession := a.sessionMgr.GetOrCreate(sessKey)
	if isNewSession || len(sess.Messages) == 0 {
		fullSystemPrompt, err := a.contextBuilder.BuildFullSystemPrompt(msg.InChannel, msg.ChatID, msg.SenderID)
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
	resp, err := a.orchestrator.Run(ctx, historyMsg)

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
	a.sessionMgr.Update(sess, currentTokenUsed)

	// TODO: 异步压缩上下文并更新summary
	go func() {
		if err = a.contextBuilder.Compact(ctx, sess, currentTokenUsed, msg.InChannel, msg.ChatID, msg.SenderID); err != nil {
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
	if err := a.bus.PublishOutBoundMessage(ctx, outbound); err != nil {
		logger.L().Error().Err(err).Msg("Error publishing response")
	}
}

// Start 启动Agent
func (a *Agent) Start(ctx context.Context) error {
	// 加载历史 sessions
	logger.L().Info().Msg("Starting agent")
	a.sessionMgr.LoadSessions()

	// 启动 session 管理
	a.sessionMgr.Start()
	// 启动 MCP 管理器并注册工具
	if err := a.mcpManager.Start(ctx, a.toolRegistry); err != nil {
		logger.L().Error().Err(err).Msg("启动 MCP 管理器失败")
		return err
	}
	return nil
}

// SaveSession 立即保存指定 session
func (a *Agent) SaveSession(sessionKey string) error {
	return a.sessionMgr.SaveSession(sessionKey)
}

// SaveAllSessions 保存所有 session
func (a *Agent) SaveAllSessions() {
	a.sessionMgr.SaveAllSessionsWithMeta()
}

// GetWorkspace 获取工作目录
func (a *Agent) GetWorkspace() string {
	return a.workspace
}

// GetMemoryDir 获取 memory 目录
func (a *Agent) GetMemoryDir() string {
	return filepath.Join(a.workspace, "memory")
}

// Stop 停止Agent
func (a *Agent) Stop() {
	// 保存所有 sessions
	a.sessionMgr.SaveAllSessionsWithMeta()

	// 停止 session manager
	a.sessionMgr.Stop()

	// 关闭 MCP 管理器，释放资源
	a.mcpManager.Stop()
}

// GetConfig 获取 Agent 配置
func (a *Agent) GetConfig() *config.AgentConfig {
	return a.config
}

// GetOrCreateSession 获取或创建会话
// 这是提供给 Gateway 的公共方法
func (a *Agent) GetOrCreateSession(key string) *Session {
	sess, _ := a.sessionMgr.GetOrCreate(key)
	return sess
}

// GetSession 获取会话（如果不存在则返回 nil）
func (a *Agent) GetSession(key string) (*Session, bool) {
	return a.sessionMgr.Get(key)
}
