package agent

import (
	"brambleclaw/bus"
	"brambleclaw/config"
	"brambleclaw/logger"
	"brambleclaw/tools"
	"brambleclaw/tools/mcp"
	"context"
	"path/filepath"
	"time"
)

// PersistentAgent 支持 session 持久化的 Agent
type PersistentAgent struct {
	config       AgentConfig
	sessionMgr   *PersistentSessionManager
	llmClient    *LLMClient
	bus          *bus.MessageBus
	toolRegistry *tools.ToolRegistry
	orchestrator *Orchestrator
	mcpManager   *mcp.Manager
	workspace    string
}

// NewPersistentAgent 创建支持持久化的 Agent
func NewPersistentAgent(agentCfg config.AgentConfig, msgBus *bus.MessageBus, workspacePath string, autosaveInterval time.Duration) *PersistentAgent {
	// 转换配置类型
	agentConfig := AgentConfig{
		Name:       agentCfg.Name,
		LLM:        agentCfg.LLM,
		MaxHistory: agentCfg.MaxHistory,
		Tools:      convertTools(agentCfg),
	}

	llmClient := NewLLMClient(agentConfig.LLM)
	toolRegistry := tools.NewToolRegistry()
	orchestrator := NewOrchestrator(llmClient, toolRegistry)

	// 创建持久化 session manager
	sessionMgr := NewPersistentSessionManager(agentConfig.Name, workspacePath, autosaveInterval)

	return &PersistentAgent{
		config:       agentConfig,
		sessionMgr:   sessionMgr,
		llmClient:    llmClient,
		bus:          msgBus,
		toolRegistry: toolRegistry,
		orchestrator: orchestrator,
		mcpManager:   mcp.NewManager(agentConfig.Tools.MCP),
		workspace:    workspacePath,
	}
}

// convertTools 转换工具配置
func convertTools(agentCfg config.AgentConfig) config.ToolsConfig {
	// 根据工具名称列表创建工具配置
	tools := config.ToolsConfig{}

	for _, name := range agentCfg.Tools {
		switch name {
		case "filesystem":
			// 文件系统工具默认启用
		case "shell":
			// Shell 工具默认启用
		case "code_sandbox":
			// 代码沙箱工具默认启用
		case "web_search":
			tools.WebSearch.Enabled = true
		case "url_parse":
			tools.UrlParse.Enabled = true
		}
	}

	return tools
}

// LoadSessions 从存储加载所有 session
func (a *PersistentAgent) LoadSessions() error {
	return a.sessionMgr.LoadSessions()
}

// RegisterTool 注册工具
func (a *PersistentAgent) RegisterTool(tool tools.Tool) {
	a.toolRegistry.Register(tool)
}

// Start 启动 Agent
func (a *PersistentAgent) Start(ctx context.Context) error {
	// 加载历史 sessions
	if err := a.LoadSessions(); err != nil {
		logger.L().Warn().Err(err).Msg("加载历史 sessions 失败")
		// 不中断启动，继续使用空 sessions
	}

	// 启动 MCP 管理器
	if err := a.mcpManager.Start(ctx, a.toolRegistry); err != nil {
		logger.L().Error().Err(err).Msg("启动 MCP 管理器失败")
		return err
	}

	return nil
}

// Stop 停止 Agent
func (a *PersistentAgent) Stop() {
	// 保存所有 sessions
	if err := a.sessionMgr.SaveAllSessions(); err != nil {
		logger.L().Error().Err(err).Msg("保存 sessions 失败")
	}

	// 停止 session manager
	a.sessionMgr.Stop()

	// 关闭 MCP 管理器
	a.mcpManager.Stop()
}

// GetOrCreateSession 获取或创建会话
func (a *PersistentAgent) GetOrCreateSession(key string) *Session {
	return a.sessionMgr.GetOrCreate(key)
}

// GetSession 获取会话
func (a *PersistentAgent) GetSession(key string) (*Session, bool) {
	return a.sessionMgr.Get(key)
}

// UpdateSession 更新会话
func (a *PersistentAgent) UpdateSession(session *Session) {
	a.sessionMgr.Update(session)
}

// Process 处理消息并返回响应
func (a *PersistentAgent) Process(ctx context.Context, history []AgentMessage) (string, error) {
	resp, err := a.orchestrator.Run(ctx, history)
	if err != nil {
		return "", err
	}
	return resp, nil
}

// GetConfig 获取 Agent 配置
func (a *PersistentAgent) GetConfig() AgentConfig {
	return a.config
}

// SaveSession 立即保存指定 session
func (a *PersistentAgent) SaveSession(sessionKey string) error {
	return a.sessionMgr.SaveSession(sessionKey)
}

// GetWorkspace 获取工作目录
func (a *PersistentAgent) GetWorkspace() string {
	return a.workspace
}

// GetMemoryDir 获取 memory 目录
func (a *PersistentAgent) GetMemoryDir() string {
	return filepath.Join(a.workspace, "memory")
}
