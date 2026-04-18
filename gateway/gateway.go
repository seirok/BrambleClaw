package gateway

import (
	"brambleclaw/config"
	"brambleclaw/logger"
	"brambleclaw/tools/mcp"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"brambleclaw/agent"
	"brambleclaw/bus"
	"brambleclaw/channel"
)

// MessageProcessor 消息处理器接口
type MessageProcessor interface {
	Process(ctx context.Context, msg *bus.InBoundMessage) error
}

// Gateway 消息网关
// 负责消息路由、Agent 管理和错误处理
type Gateway struct {
	config         *GatewayConfig
	router         *Router
	registry       *AgentRegistry
	channelManager *channel.Manager
	msgBus         *bus.MessageBus

	// 运行时状态
	mu      sync.RWMutex
	running bool
	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
}

// NewGateway 创建新的 Gateway 实例
func NewGateway(
	config *GatewayConfig,
	msgBus *bus.MessageBus,
	channelManager *channel.Manager,
) *Gateway {
	registry := NewAgentRegistry()
	router := NewRouter(config, registry)

	return &Gateway{
		config:         config,
		router:         router,
		registry:       registry,
		channelManager: channelManager,
		msgBus:         msgBus,
		running:        false,
	}
}

// RegisterAgent 注册 Agent 到 Gateway
func (g *Gateway) RegisterAgents(config *config.Config) error {
	mcpManager := mcp.NewManager(config.Tools.MCP)

	// 标记是否有配置被修改，需要同步到 config.json
	configModified := false

	for i := range config.Agents {
		cfg := &config.Agents[i]
		if !cfg.Enabled {
			continue
		}

		// 检查并修正 agent 的 workspace 路径
		// 正确的路径应该为： config.Workspace/cfg.Name/cfg.Workspace
		expectedWorkspace := filepath.Join(config.Workspace, cfg.Name, cfg.Workspace)

		// 检查当前路径是否与期望路径一致
		if cfg.Workspace != expectedWorkspace {
			logger.L().Debug().
				Str("Agent", cfg.Name).
				Str("OldWorkspace", cfg.Workspace).
				Str("NewWorkspace", expectedWorkspace).
				Msg("修正 agent workspace 路径")

			cfg.Workspace = expectedWorkspace
			configModified = true
		}

		contextBuilder, err := agent.NewContextBuilder(&config.Compact)
		if err != nil {
			return err
		}

		newAgent := agent.NewAgent(cfg, g.msgBus, mcpManager, contextBuilder)
		if err := g.registry.Register(cfg.Name, newAgent, cfg); err != nil {
			return err
		}
		logger.L().Debug().Str("Agent", cfg.Name).Msg("register agent")
	}

	// 如果有配置被修改，同步到 config.json
	if configModified {
		if err := g.saveConfigToFile(config); err != nil {
			logger.L().Error().Err(err).Msg("[RegisterAgents] 保存配置到 config.json 失败")
			// 继续执行，不因为保存失败而中断注册流程
		}
	}

	if len(g.registry.List()) == 0 {
		return fmt.Errorf("[RegisterAgents]No available agents")
	}

	return nil
}

// Start 启动 Gateway
func (g *Gateway) Start(ctx context.Context) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.running {
		return fmt.Errorf("Gateway 已处于运行状态")
	}

	g.ctx, g.cancel = context.WithCancel(ctx)
	g.running = true

	// 启动消息处理循环
	g.wg.Add(1)
	go g.processMessageLoop()

	// 启动出站消息分发
	g.wg.Add(1)
	go g.dispatchOutboundLoop()

	// 启动健康检查（如果启用）
	if g.config.HealthCheck.Enabled {
		g.wg.Add(1)
		go g.healthCheckLoop()
	}

	logger.L().Info().Msg("[Gateway] 已启动")
	return nil
}

// Stop 停止 Gateway
func (g *Gateway) Stop() {
	g.mu.Lock()
	if !g.running {
		g.mu.Unlock()
	}
	g.running = false
	g.cancel()
	g.mu.Unlock()

	// 等待所有 goroutine 结束
	done := make(chan struct{})
	go func() {
		g.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		logger.L().Info().Msg("[Gateway] 已正常停止")
	case <-time.After(10 * time.Second):
		logger.L().Warn().Msg("[Gateway] 停止超时，强制结束")
	}

}

// processMessageLoop 消息处理主循环
func (g *Gateway) processMessageLoop() {
	defer g.wg.Done()

	logger.L().Debug().Msg("[Gateway] 消息处理循环启动")
	for {
		select {
		case <-g.ctx.Done():
			return
		default:
		}

		// 消费入站消息
		msg, err := g.msgBus.ConsumeInBoundMessage(g.ctx) // 阻塞等待
		if err != nil {
			if g.ctx.Err() != nil {
				return
			}
			logger.L().Error().Err(err).Msg("[Gateway] 消费消息失败")
			continue
		}

		// 处理消息
		if err := g.handleMessage(g.ctx, msg); err != nil {
			logger.L().Error().Err(err).Msg("[Gateway] 处理消息失败")
		}
	}
}

// handleMessage 处理单条消息
func (g *Gateway) handleMessage(ctx context.Context, msg *bus.InBoundMessage) error {
	// 1. 解析路由
	route, err := g.router.ResolveRoute(msg)
	if err != nil {
		return fmt.Errorf("路由解析失败: %w", err)
	}

	// 2. 获取 Agent
	agentEntry, exists := g.registry.Get(route.AgentName)
	if !exists {
		return fmt.Errorf("Agent不存在(name=%s)", route.AgentName)
	}

	// Agent 处理消息
	agentEntry.Agent.HandleMessage(ctx, msg)
	return nil
}

// dispatchOutboundLoop 出站消息分发循环
func (g *Gateway) dispatchOutboundLoop() {
	defer g.wg.Done()

	// 启动订阅消息分发
	logger.L().Debug().Msg("[Gateway] 订阅分发启动")
	go g.msgBus.DistributeOutBoundMessage(g.ctx)

	logger.L().Debug().Msg("[Gateway] 响应分发启动")
	sub := g.msgBus.Subscribe()
	defer g.msgBus.Unsubscribe(sub.ID)

	for {
		select {
		case <-g.ctx.Done():
			return
		case msg, ok := <-sub.Channel:
			if !ok {
				return
			}

			// 通过 Channel Manager 发送消息
			ch, exists := g.channelManager.Get(msg.OutChannel)
			if !exists {
				logger.L().Error().Str("Channel", msg.OutChannel).Msg("[Gateway] 通道不存在")
				continue
			}

			if err := ch.Send(msg); err != nil {
				logger.L().Error().Err(err).Str("Channel", msg.OutChannel).Msg("[Gateway] 发送消息失败")
			}
		}
	}
}

// healthCheckLoop 健康检查循环
func (g *Gateway) healthCheckLoop() {
	defer g.wg.Done()

	ticker := time.NewTicker(time.Duration(g.config.HealthCheck.Interval) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-g.ctx.Done():
			return
		case <-ticker.C:
			g.performHealthCheck()
		}
	}
}

// performHealthCheck 执行健康检查
func (g *Gateway) performHealthCheck() {
	// 检查所有通道状态
	// 这里可以实现具体的健康检查逻辑
	// logger.L().Info().Msg("[Gateway] 执行健康检查")
}

// IsRunning 检查 Gateway 是否正在运行
func (g *Gateway) IsRunning() bool {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.running
}

// GetRegistry 获取 Agent 注册表
func (g *Gateway) GetRegistry() *AgentRegistry {
	return g.registry
}

// GetConfig 获取 Gateway 配置
func (g *Gateway) GetConfig() *GatewayConfig {
	return g.config
}

// saveConfigToFile 将配置保存到 config.json 文件
// 查找并更新现有的配置文件路径
func (g *Gateway) saveConfigToFile(cfg *config.Config) error {
	loader := config.NewLoader()
	_, configPath, err := loader.Load()
	if err != nil {
		return fmt.Errorf("查找配置文件失败: %w", err)
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("JSON序列化失败: %w", err)
	}

	if err := os.WriteFile(configPath, data, 0644); err != nil {
		return fmt.Errorf("写入配置文件失败(%s): %w", configPath, err)
	}

	logger.L().Debug().Str("path", configPath).Msg("[Gateway] 配置已同步到文件")
	return nil
}
