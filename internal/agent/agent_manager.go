package agent

import (
	"context"
	"fmt"
	util "neoclaw/internal"
	"neoclaw/internal/audit"
	"neoclaw/internal/bus"
	"neoclaw/internal/command"
	"neoclaw/internal/config"
	"neoclaw/internal/cron"
	"neoclaw/internal/interfaces"
	"neoclaw/internal/logger"
	"neoclaw/internal/messages"
	"neoclaw/internal/sandbox"
	"neoclaw/internal/session"
	"neoclaw/internal/skill"
	"neoclaw/internal/store"
	"neoclaw/internal/tools"
	"neoclaw/internal/tools/mcp"
	"path/filepath"
	"sync"
)

type AgentManager struct {
	//	currentAgent  *Agent
	agentRegistry interfaces.Registry[*Agent]
	toolRegistry  interfaces.Registry[interfaces.Tool]
	msgBus        *bus.MessageBus
	runtime       messages.RuntimeProvider
	skillManager  *skill.SkillManager // Will be *skill.SkillManager
	mu            sync.RWMutex
	status        interfaces.ManagerStatus
	auditLoggers  []*audit.AuditLogger
	cronService   *cron.CronService

	// toolFactory 注入额外的工具到每个 Agent
	toolFactory func(*Agent) []interfaces.Tool
	// commandFactory 注入额外的命令到每个 Agent（由外部设置，避免循环依赖）
	commandFactory func(*Agent) []interfaces.Command
}

func NewAgentManager(msgBus *bus.MessageBus, rt messages.RuntimeProvider) *AgentManager {
	return &AgentManager{
		agentRegistry: NewAgentRegistry(),
		msgBus:        msgBus,
		runtime:       rt,
	}
}

// SetToolFactory 设置工具工厂，用于向每个 Agent 注入额外工具
func (a *AgentManager) SetToolFactory(factory func(*Agent) []interfaces.Tool) {
	a.toolFactory = factory
}

// SetCommandFactory 设置命令工厂，用于向每个 Agent 注入额外命令
func (a *AgentManager) SetCommandFactory(factory func(*Agent) []interfaces.Command) {
	a.commandFactory = factory
}

// SetSkillManager 设置技能管理器（使用 interface{} 避免导入循环）
func (a *AgentManager) SetSkillManager(sm *skill.SkillManager) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.skillManager = sm
}

// Initialize 从配置中注册所有启用的 Agent
func (a *AgentManager) Initialize(ctx context.Context, cfg any) error {
	fullCfg, ok := cfg.(*config.Config)
	if !ok {
		return fmt.Errorf("invalid config type for AgentManager")
	}

	a.mu.Lock()
	a.status = interfaces.StatusRunning
	a.mu.Unlock()

	// MCP Manager
	mcpManager := mcp.NewManager(fullCfg.Tools.MCP)

	// Initialize SkillManager if already set
	if fullCfg.Skill.Enabled && a.skillManager != nil {
		if err := a.skillManager.Initialize(ctx, &fullCfg.Skill); err != nil {
			logger.L().Warn().Err(err).Msg("Failed to initialize SkillManager")
		}
	}

	for i := range fullCfg.Agents {
		agentCfg := &fullCfg.Agents[i]
		if !agentCfg.Enabled {
			continue
		}

		// Build agent instance
		contextBuilder, err := NewContextBuilder(&fullCfg.Compact)
		if err != nil {
			return fmt.Errorf("创建 context builder 失败: %w", err)
		}

		// Tool registry
		llmClient := NewLLMClient(fullCfg.LLMConfig)

		// Initialize summary compressor
		contextBuilder.SetSummaryCompressor(NewSummaryCompressor(fullCfg.Compact, llmClient))

		// Session
		agentWorkspace := filepath.Join(util.GetSystemPath(), agentCfg.Name)

		// 每个 Agent 创建独立的审计日志
		auditLogPath := filepath.Join(agentWorkspace, "audit.log")
		agentAuditLogger, err := audit.NewAuditLoggerWithPath(fullCfg.Sandbox.Audit, auditLogPath)
		if err != nil {
			logger.L().Warn().Err(err).Str("agent", agentCfg.Name).Msg("Failed to initialize AuditLogger")
			agentAuditLogger = nil
		}
		if agentAuditLogger != nil {
			a.auditLoggers = append(a.auditLoggers, agentAuditLogger)
		}

		websearchTool := tools.NewWebSearchTool(fullCfg.Tools.WebSearch.APIKey)
		urlparsetool := tools.NewUrlParseTool()
		sandBox, err := sandbox.NewSandbox(&fullCfg.Sandbox, agentAuditLogger)
		if err != nil {
			return fmt.Errorf("创建 Sandbox 失败: %w", err)
		}
		shellTool := sandbox.NewShellTool(sandBox)
		globTool := tools.NewGlobTool()
		grepTool := tools.NewGrepTool()
		readTool := tools.NewReadTool(sandBox)
		writeTool := tools.NewWriteTool(sandBox)
		listTool := tools.NewListTool(sandBox)
		grantPermTool := tools.NewGrantPermissionTool(sandBox)
		a.toolRegistry = tools.NewToolRegistry()
		if err := a.toolRegistry.Register(ctx, "web_search", websearchTool); err != nil {
			return fmt.Errorf("注册 web_search 工具失败: %w", err)
		}
		if err := a.toolRegistry.Register(ctx, "shell", shellTool); err != nil {
			return fmt.Errorf("注册 shell 工具失败: %w", err)
		}

		if err := a.toolRegistry.Register(ctx, "url_parse", urlparsetool); err != nil {
			return fmt.Errorf("注册 url_parse 工具失败: %w", err)
		}
		if err := a.toolRegistry.Register(ctx, "glob", globTool); err != nil {
			return fmt.Errorf("注册 glob 工具失败: %w", err)
		}
		if err := a.toolRegistry.Register(ctx, "grep", grepTool); err != nil {
			return fmt.Errorf("注册 grep 工具失败: %w", err)
		}
		if err := a.toolRegistry.Register(ctx, "read", readTool); err != nil {
			return fmt.Errorf("注册 read 工具失败: %w", err)
		}
		if err := a.toolRegistry.Register(ctx, "write", writeTool); err != nil {
			return fmt.Errorf("注册 write 工具失败: %w", err)
		}
		if err := a.toolRegistry.Register(ctx, "list", listTool); err != nil {
			return fmt.Errorf("注册 list 工具失败: %w", err)
		}
		if err := a.toolRegistry.Register(ctx, "grant_permission", grantPermTool); err != nil {
			return fmt.Errorf("注册 grant_permission 工具失败: %w", err)
		}

		agentToolRegistry := tools.NewToolRegistry()
		for _, toolName := range agentCfg.Tools {
			tool, err := a.toolRegistry.Get(ctx, toolName)
			if err != nil {
				logger.L().Error().Err(err).Str("tool", toolName).Msg("Invalid Agent Tool configuration")
				continue
			}
			if err := agentToolRegistry.Register(ctx, toolName, tool); err != nil {
				return fmt.Errorf("注册 Agent Tool %s 失败: %w", toolName, err)
			}
			logger.L().Debug().Str("tool", toolName).Msg("Agent Tool registered successfully")
		}

		// Orchestrator
		orche := NewOrchestrator(llmClient, agentToolRegistry, agentAuditLogger)

		// Build command registry
		cmdRegistry := command.NewCommandRegistry()
		if err := cmdRegistry.Register(ctx, "clear", &command.ClearCommand{}); err != nil {
			logger.L().Error().Err(err).Msg("Failed to register clear command")
		}
		if err := cmdRegistry.Register(ctx, "help", &command.HelpCommand{}); err != nil {
			logger.L().Error().Err(err).Msg("Failed to register help command")
		}
		if err := cmdRegistry.Register(ctx, "reset", &command.ResetCommand{}); err != nil {
			logger.L().Error().Err(err).Msg("Failed to register reset command")
		}
		if err := cmdRegistry.Register(ctx, "compact", &command.CompactCommand{}); err != nil {
			logger.L().Error().Err(err).Msg("Failed to register compact command")
		}
		if err := cmdRegistry.Register(ctx, "undo", &command.UndoCommand{}); err != nil {
			logger.L().Error().Err(err).Msg("Failed to register undo command")
		}
		if err := cmdRegistry.Register(ctx, "model", &command.ModelCommand{}); err != nil {
			logger.L().Error().Err(err).Msg("Failed to register model command")
		}
		if err := cmdRegistry.Register(ctx, "skills", &command.SkillsCommand{}); err != nil {
			logger.L().Error().Err(err).Msg("Failed to register skills command")
		}
		if err := cmdRegistry.Register(ctx, "context", &command.ContextCommand{}); err != nil {
			logger.L().Error().Err(err).Msg("Failed to register context command")
		}

		// Session
		sessionStore := store.NewFileStorage[session.Session](filepath.Join(agentWorkspace, "memory"))
		metaStore := store.NewFileStorage[session.SessionMetadata](filepath.Join(agentWorkspace, "memory", "meta_data"))
		sess := session.NewSession(session.WithStores(sessionStore, metaStore)) // session key 需要由对话确定

		// Set skill manager on agent
		agent := NewAgent(agentCfg.Name,
			WithContextBuilder(contextBuilder),
			WithBus(a.msgBus),
			WithMCP(mcpManager),
			WithTools(agentToolRegistry),
			WithCommands(cmdRegistry),
			WithOrchestrator(orche),
			WithDescription(agentCfg.Description),
			WithRuntime(a.runtime),
			WithWorkspace(agentWorkspace),
			WithSkillManager(a.skillManager),
			WithSession(sess),
		)

		// agent 应该拥有写自己的 memory.md 的权限
		config.Get().Sandbox.FileSystem.AllowWritePaths = append(config.Get().Sandbox.FileSystem.AllowWritePaths,
			filepath.Join(agentWorkspace, "memory"))

		// Initialize workspace skills
		if err := a.skillManager.AddWorkspace(ctx, agentWorkspace); err != nil {
			logger.L().Warn().Err(err).Str("agent", agentCfg.Name).Msg("Failed to add workspace to SkillManager")
		}

		// Set agent on ContextBuilder
		contextBuilder.SetAgent(agent)

		// Inject extra tools via factory
		if a.toolFactory != nil {
			for _, tool := range a.toolFactory(agent) {
				if err := agentToolRegistry.Register(ctx, tool.Name(), tool); err != nil {
					logger.L().Error().Err(err).Str("tool", tool.Name()).Msg("Failed to register factory tool")
				}
			}
		}

		// Inject extra commands via factory
		if a.commandFactory != nil {
			for _, cmd := range a.commandFactory(agent) {
				if err := cmdRegistry.Register(ctx, cmd.Name(), cmd); err != nil {
					logger.L().Error().Err(err).Str("command", cmd.Name()).Msg("Failed to register factory command")
				}
			}
		}

		if err := a.agentRegistry.Register(ctx, agentCfg.Name, agent); err != nil {
			logger.L().Error().Err(err).Str("agent", agentCfg.Name).Msg("Failed to register Agent")
			continue
		}

	}

	// 设置当前 agent
	//defaultAgent, err := a.agentRegistry.Get(ctx, interfaces.DefaultAgentName)
	//if err != nil {
	//	return err
	//}
	//
	//a.currentAgent = defaultAgent

	a.mu.Lock()
	a.status = interfaces.StatusRunning
	a.mu.Unlock()

	return nil
}

func (a *AgentManager) registerSkillCommands(ctx context.Context, agent *Agent) {
	// Register slash commands for user-invocable skills (temporarily disabled)
}

// Get 包装底层 Registry 的获取逻辑
func (a *AgentManager) Get(ctx context.Context, id string) (*Agent, error) {
	return a.agentRegistry.Get(ctx, id)
}

// Add 手动添加一个 Agent
func (a *AgentManager) Add(ctx context.Context, id string, item *Agent) error {
	return a.agentRegistry.Register(ctx, id, item)
}

// Remove 移除并停止 Agent
func (a *AgentManager) Remove(ctx context.Context, id string) error {
	return a.agentRegistry.Unregister(ctx, id)
}

// List 获取所有实例列表
func (a *AgentManager) List(ctx context.Context) []*Agent {
	return a.agentRegistry.List(ctx)
}

// StartAll 启动所有 Agent
func (a *AgentManager) StartAll(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	// 启动 CronService
	if a.cronService != nil {
		if err := a.cronService.Start(ctx); err != nil {
			logger.L().Error().Err(err).Msg("Failed to start CronService")
		}
	}

	var errs []error
	agents := a.agentRegistry.List(ctx)
	for _, agent := range agents {
		if err := agent.Start(ctx); err != nil {
			errs = append(errs, fmt.Errorf("failed to start agent %q: %w", agent.Name(), err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("errors starting agents: %v", errs)
	}

	a.status = interfaces.StatusRunning
	return nil
}

// StopAll 停止所有 Agent
func (a *AgentManager) StopAll(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.status == interfaces.StatusStopped {
		return nil
	}

	// 停止 CronService
	if a.cronService != nil {
		if err := a.cronService.Stop(ctx); err != nil {
			logger.L().Error().Err(err).Msg("Failed to stop CronService")
		}
	}

	var errs []error
	agents := a.agentRegistry.List(ctx)
	for _, agent := range agents {
		if err := agent.Stop(ctx); err != nil {
			errs = append(errs, fmt.Errorf("failed to stop agent %q: %w", agent.Name(), err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("errors stopping agents: %v", errs)
	}
	// 关闭所有审计日志记录器
	for _, auditLogger := range a.auditLoggers {
		if auditLogger != nil {
			if err := auditLogger.Close(); err != nil {
				logger.L().Warn().Err(err).Msg("Failed to close AuditLogger")
			}
		}
	}

	a.status = interfaces.StatusStopped
	return nil
}

func (a *AgentManager) Status() interfaces.ManagerStatus {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.status
}
