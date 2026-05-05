package agent

import (
	util "brambleclaw/internal"
	"brambleclaw/internal/bus"
	"brambleclaw/internal/config"
	"brambleclaw/internal/interfaces"
	"brambleclaw/internal/logger"
	"brambleclaw/internal/sandbox"
	"brambleclaw/internal/session"
	"brambleclaw/internal/tools"
	"brambleclaw/internal/tools/mcp"
	"context"
	"fmt"
	"path/filepath"
	"sync"
)

type AgentManager struct {
	agentRegistry interfaces.Registry[*Agent]
	toolRegistry  interfaces.Registry[tools.Tool]
	msgBus        *bus.MessageBus
	mu            sync.RWMutex
	status        interfaces.ManagerStatus
}

func NewAgentManager(msgBus *bus.MessageBus) *AgentManager {
	return &AgentManager{
		agentRegistry: NewAgentRegistry(),
		msgBus:        msgBus,
		status:        interfaces.StatusIdle,
	}
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

	// 遍历配置并注册 Agent
	for i := range fullCfg.Agents {
		agentCfg := &fullCfg.Agents[i]
		if !agentCfg.Enabled {
			continue
		}

		// 构建 Agent 实例
		// context builder
		contextBuilder, err := NewContextBuilder(&fullCfg.Compact)
		if err != nil {
			return fmt.Errorf("创建 context builder 失败: %w", err)
		}

		// tool registry
		llmClient := NewLLMClient(fullCfg.LLMConfig)
		websearchTool := tools.NewWebSearchTool(fullCfg.Tools.WebSearch.APIKey)
		urlparsetool := tools.NewUrlParseTool()
		sandBox, err := sandbox.NewSandbox(&fullCfg.Sandbox, nil)
		if err != nil {
			logger.L().Error().Err(err).Msg("")
		}
		filesystemTool := sandbox.NewFileSystemTool(sandBox)
		shellTool := sandbox.NewShellTool(sandBox)
		a.toolRegistry = tools.NewToolRegistry()
		if err := a.toolRegistry.Register(ctx, "web_search", websearchTool); err != nil {
			logger.L().Error().Err(err).Msg("注册 web_search 工具失败")
		}
		if err := a.toolRegistry.Register(ctx, "shell", shellTool); err != nil {
			logger.L().Error().Err(err).Msg("注册 shell 工具失败")
		}
		if err := a.toolRegistry.Register(ctx, "filesystem", filesystemTool); err != nil {
			logger.L().Error().Err(err).Msg("注册 filesystem 工具失败")
		}
		if err := a.toolRegistry.Register(ctx, "url_parse", urlparsetool); err != nil {
			logger.L().Error().Err(err).Msg("注册 url_parse 工具失败")
		}

		agentToolRegistry := tools.NewToolRegistry()
		for _, tool := range agentCfg.Tools {
			toolTerm, err := a.toolRegistry.Get(ctx, tool)
			if err != nil {
				logger.L().Error().Err(err).Msg("不合理的Agent Tool 配置项")
			}
			if err = agentToolRegistry.Register(ctx, tool, toolTerm); err != nil {
				logger.L().Error().Err(err).Msg("")
			}
		}

		// Orchestrator
		orche := NewOrchestrator(llmClient, agentToolRegistry)
		// session manager
		agentWorkspace := filepath.Join(util.GetSystemPath(), agentCfg.Name)
		sm := session.NewPersistentSessionManager(agentWorkspace)

		// 构建 Agent 实例
		agent := NewAgent(agentCfg.Name,
			WithContextBuilder(contextBuilder),
			WithBus(a.msgBus),
			WithMCP(mcpManager),
			WithTools(agentToolRegistry),
			WithOrchestrator(orche),
			WithSessionManager(sm),
			WithDescription(agentCfg.Description),
		)

		// 将 Agent 设置到 ContextBuilder（解决循环依赖）
		contextBuilder.SetAgent(agent)

		//
		if err := a.agentRegistry.Register(ctx, agent.Name(), agent); err != nil {
			logger.L().Error().Err(err).Str("agent", agent.Name()).Msg("注册 Agent 失败")
			continue
		}
	}

	// 检查是否至少有一个可用 agent
	if len(a.agentRegistry.List(ctx)) == 0 {
		return fmt.Errorf("没有可用的 agent")
	}

	a.mu.Lock()
	a.status = interfaces.StatusRunning
	a.mu.Unlock()

	return nil
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
	// 可以在这里增加优雅关闭逻辑
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

	a.status = interfaces.StatusStopped
	return nil
}

func (a *AgentManager) Status() interfaces.ManagerStatus {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.status
}
