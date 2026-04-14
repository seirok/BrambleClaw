package agent

import (
	"context"
	"log"
	"miniGoClaw/bus"
	"miniGoClaw/config"
	"miniGoClaw/logger"
	"miniGoClaw/tools"
	"miniGoClaw/tools/mcp"
	"strings"
	"time"
)

// AgentConfig Agent配置
type AgentConfig struct {
	Name       string             `json:"name"`
	LLM        config.LLMConfig   `json:"llm"`
	MaxHistory int                `json:"max_history"`
	Tools      config.ToolsConfig `json:"tools"`
}

// Agent Agent核心
type Agent struct {
	config       AgentConfig
	sessionMgr   *SessionManager
	llmClient    *LLMClient
	bus          *bus.MessageBus
	toolRegistry *tools.ToolRegistry
	orchestrator *Orchestrator
	mcpManager   *mcp.Manager
}

// NewAgent 创建Agent
func NewAgent(config AgentConfig, bus *bus.MessageBus) *Agent {
	llmClient := NewLLMClient(config.LLM)
	toolRegistry := tools.NewToolRegistry()
	orchestrator := NewOrchestrator(llmClient, toolRegistry)
	return &Agent{
		config:       config,
		sessionMgr:   NewSessionManager(),
		llmClient:    llmClient,
		bus:          bus,
		toolRegistry: toolRegistry,
		orchestrator: orchestrator,
		mcpManager:   mcp.NewManager(config.Tools.MCP),
	}
}

func (a *Agent) RegisterTool(tool tools.Tool) {
	a.toolRegistry.Register(tool)
}

func shouldExecuteTool(content string) bool {
	// 简单的工具调用检测
	return strings.HasPrefix(content, "/tool")
}

// handleInboundMessage 处理入站消息
func (a *Agent) handleInboundMessage(ctx context.Context, msg *bus.InBoundMessage) {
	// 当前消息转化为AgentMessage格式
	agentMsg := AgentMessage{
		Role:      RoleUser,
		Content:   []ContentBlock{TextContent{Text: msg.Content}},
		Timestamp: time.Now().UnixMilli(),
	}

	// 添加当前消息到历史消息
	sessKey := msg.SessionKey()
	sess := a.sessionMgr.GetOrCreate(sessKey)
	sess.Messages = append(sess.Messages, agentMsg)

	// 加载历史消息
	historyMsg := sess.GetHistory(a.config.MaxHistory)

	// 使用Orchestrator处理消息
	resp, err := a.orchestrator.Run(ctx, historyMsg)

	if err != nil {
		log.Println("Failed to complete communication with LLM:", err)
		return
	}

	replyMsg := AgentMessage{
		Role:      RoleAssistant,
		Content:   []ContentBlock{TextContent{Text: resp}},
		Timestamp: time.Now().UnixMilli(),
	}
	// 添加回复到会话
	sess.AddMessage(replyMsg)

	// 更新会话
	a.sessionMgr.Update(sess)

	// 发布响应
	outbound := &bus.OutBoundMessage{
		OutChannel: msg.InChannel,
		ChatID:     msg.ChatID,
		Content:    resp,
		ReplyTo:    msg.ID,
		TimeStamp:  time.Now(),
	}

	if err := a.bus.PublishOutBoundMessage(ctx, outbound); err != nil {
		log.Printf("Error publishing response: %v", err)
	}

}

// processMessages 处理消息
func (a *Agent) processMessages(ctx context.Context) {
	for {
		msg, err := a.bus.ConsumeInBoundMessage(ctx)
		if err != nil {
			log.Printf("Error consuming message: %v", err)
			continue
		}

		// 处理消息
		a.handleInboundMessage(ctx, msg)

	}
}

// Start 启动Agent
func (a *Agent) Start(ctx context.Context) error {
	// 启动 MCP 管理器并注册工具
	if err := a.mcpManager.Start(ctx, a.toolRegistry); err != nil {
		logger.L().Error().Err(err).Msg("启动 MCP 管理器失败")
		return err
	}

	// 注意：如果使用 Gateway 路由消息，这里不应该启动独立的 processMessages 循环，
	// 因为 Gateway 会消费消息并调用 a.Process()。
	// 但为了兼容独立运行模式，这里暂且保留或由上层决定是否启动。
	// 为了避免和 Gateway 争抢消息，当作为 Gateway 的 Agent 注册时，不应调用此处的 processMessages。
	return nil
}

// StartStandalone 独立模式启动Agent（自己消费消息）
func (a *Agent) StartStandalone(ctx context.Context) error {
	if err := a.Start(ctx); err != nil {
		return err
	}
	go a.processMessages(ctx)
	return nil
}

// Stop 停止Agent
func (a *Agent) Stop() {
	// 关闭 MCP 管理器，释放资源
	a.mcpManager.Stop()
}

// GetConfig 获取 Agent 配置
func (a *Agent) GetConfig() AgentConfig {
	return a.config
}

// GetOrCreateSession 获取或创建会话
// 这是提供给 Gateway 的公共方法
func (a *Agent) GetOrCreateSession(key string) *Session {
	return a.sessionMgr.GetOrCreate(key)
}

// GetSession 获取会话（如果不存在则返回 nil）
func (a *Agent) GetSession(key string) (*Session, bool) {
	return a.sessionMgr.Get(key)
}

// UpdateSession 更新会话
func (a *Agent) UpdateSession(session *Session) {
	a.sessionMgr.Update(session)
}

// Process 处理消息并返回响应
// 这是提供给 Gateway 的同步处理方法
func (a *Agent) Process(ctx context.Context, history []AgentMessage) (string, error) {
	resp, err := a.orchestrator.Run(ctx, history)
	if err != nil {
		return "", err
	}
	return resp, nil
}
