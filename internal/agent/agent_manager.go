package agent

import (
	util "brambleclaw/internal"
	"brambleclaw/internal/bus"
	"brambleclaw/internal/command"
	"brambleclaw/internal/config"
	"brambleclaw/internal/interfaces"
	"brambleclaw/internal/logger"
	"brambleclaw/internal/messages"
	"brambleclaw/internal/sandbox"
	"brambleclaw/internal/session"
	"brambleclaw/internal/skill"
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
	runtime       messages.RuntimeProvider
	skillManager  *skill.SkillManager // Will be *skill.SkillManager
	mu            sync.RWMutex
	status        interfaces.ManagerStatus

	// toolFactory injects extra tools to each Agent
	toolFactory func(*Agent) []tools.Tool
	// commandFactory injects extra commands to each Agent (set externally to avoid circular dependency)
	commandFactory func(*Agent) []interfaces.Command
}

func NewAgentManager(msgBus *bus.MessageBus, rt messages.RuntimeProvider) *AgentManager {
	return &AgentManager{
		agentRegistry: NewAgentRegistry(),
		msgBus:        msgBus,
		runtime:       rt,
	}
}

// SetToolFactory sets tool factory for injecting extra tools to each Agent
func (a *AgentManager) SetToolFactory(factory func(*Agent) []tools.Tool) {
	a.toolFactory = factory
}

// SetCommandFactory sets command factory for injecting extra commands to each Agent
func (a *AgentManager) SetCommandFactory(factory func(*Agent) []interfaces.Command) {
	a.commandFactory = factory
}

// SetSkillManager sets skill manager (uses interface{} to avoid import cycle)
func (a *AgentManager) SetSkillManager(sm *skill.SkillManager) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.skillManager = sm
}

// Initialize registers all enabled Agents from config
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
			return fmt.Errorf("failed to create context builder: %w", err)
		}

		// Tool registry
		llmClient := NewLLMClient(fullCfg.LLMConfig)
		websearchTool := tools.NewWebSearchTool(fullCfg.Tools.WebSearch.APIKey)
		urlparsetool := tools.NewUrlParseTool()
		sandBox, err := sandbox.NewSandbox(&fullCfg.Sandbox, nil)
		if err != nil {
			return fmt.Errorf("failed to create Sandbox: %w", err)
		}
		filesystemTool := sandbox.NewFileSystemTool(sandBox)
		shellTool := sandbox.NewShellTool(sandBox)
		globTool := tools.NewGlobTool()
		grepTool := tools.NewGrepTool()
		readTool := tools.NewReadTool(sandBox)
		writeTool := tools.NewWriteTool(sandBox)
		a.toolRegistry = tools.NewToolRegistry()
		if err := a.toolRegistry.Register(ctx, "web_search", websearchTool); err != nil {
			return fmt.Errorf("failed to register web_search tool: %w", err)
		}
		if err := a.toolRegistry.Register(ctx, "shell", shellTool); err != nil {
			return fmt.Errorf("failed to register shell tool: %w", err)
		}
		if err := a.toolRegistry.Register(ctx, "filesystem", filesystemTool); err != nil {
			return fmt.Errorf("failed to register filesystem tool: %w", err)
		}
		if err := a.toolRegistry.Register(ctx, "url_parse", urlparsetool); err != nil {
			return fmt.Errorf("failed to register url_parse tool: %w", err)
		}
		if err := a.toolRegistry.Register(ctx, "glob", globTool); err != nil {
			return fmt.Errorf("failed to register glob tool: %w", err)
		}
		if err := a.toolRegistry.Register(ctx, "grep", grepTool); err != nil {
			return fmt.Errorf("failed to register grep tool: %w", err)
		}
		if err := a.toolRegistry.Register(ctx, "read", readTool); err != nil {
			return fmt.Errorf("failed to register read tool: %w", err)
		}
		if err := a.toolRegistry.Register(ctx, "write", writeTool); err != nil {
			return fmt.Errorf("failed to register write tool: %w", err)
		}

		agentToolRegistry := tools.NewToolRegistry()
		for _, toolName := range agentCfg.Tools {
			tool, err := a.toolRegistry.Get(ctx, toolName)
			if err != nil {
				logger.L().Error().Err(err).Str("tool", toolName).Msg("Invalid Agent Tool configuration")
				continue
			}
			if err := agentToolRegistry.Register(ctx, toolName, tool); err != nil {
				return fmt.Errorf("failed to register Agent Tool %s: %w", toolName, err)
			}
			logger.L().Debug().Str("tool", toolName).Msg("Agent Tool registered successfully")
		}

		// Orchestrator
		orche := NewOrchestrator(llmClient, agentToolRegistry)
		// Session manager
		agentWorkspace := filepath.Join(util.GetSystemPath(), agentCfg.Name)
		sm := session.NewPersistentSessionManager(agentWorkspace)

		// Build command registry
		cmdRegistry := command.NewCommandRegistry()
		if err := cmdRegistry.Register(ctx, "clear", &command.ClearCommand{}); err != nil {
			logger.L().Error().Err(err).Msg("Failed to register clear command")
		}

		// Set skill manager on agent
		agent := NewAgent(agentCfg.Name,
			WithContextBuilder(contextBuilder),
			WithBus(a.msgBus),
			WithMCP(mcpManager),
			WithTools(agentToolRegistry),
			WithCommands(cmdRegistry),
			WithOrchestrator(orche),
			WithSessionManager(sm),
			WithDescription(agentCfg.Description),
			WithRuntime(a.runtime),
			WithWorkspace(agentWorkspace),
			WithSkillManager(a.skillManager),
		)

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

	// Check at least one agent is available
	if len(a.agentRegistry.List(ctx)) == 0 {
		return fmt.Errorf("no available Agent")
	}

	a.mu.Lock()
	a.status = interfaces.StatusRunning
	a.mu.Unlock()

	return nil
}

func (a *AgentManager) registerSkillCommands(ctx context.Context, agent *Agent) {
	// Register slash commands for user-invocable skills (temporarily disabled)
}

// Get wraps underlying Registry get logic
func (a *AgentManager) Get(ctx context.Context, id string) (*Agent, error) {
	return a.agentRegistry.Get(ctx, id)
}

// Add manually adds an Agent
func (a *AgentManager) Add(ctx context.Context, id string, item *Agent) error {
	return a.agentRegistry.Register(ctx, id, item)
}

// Remove removes and stops an Agent
func (a *AgentManager) Remove(ctx context.Context, id string) error {
	return a.agentRegistry.Unregister(ctx, id)
}

// List gets all instance list
func (a *AgentManager) List(ctx context.Context) []*Agent {
	return a.agentRegistry.List(ctx)
}

// StartAll starts all Agents
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

// StopAll stops all Agents
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
