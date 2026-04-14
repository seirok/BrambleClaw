package gateway

import (
	"brambleclaw/logger"
	"context"
	"fmt"
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
// 中间层职责：透传错误
func (g *Gateway) RegisterAgent(name string, ag *agent.Agent, config agent.AgentConfig) error {
	if err := g.registry.Register(name, ag, config); err != nil {
		return err
	}
	logger.L().Info().Str("Agent", name).Msg("[Gateway] Agent 已注册")
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
func (g *Gateway) Stop() error {
	g.mu.Lock()
	if !g.running {
		g.mu.Unlock()
		return nil
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

	return nil
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

	// 3. 获取或创建会话
	sess := agentEntry.Agent.GetOrCreateSession(route.SessionKey)

	// 4. 构建 AgentMessage
	agentMsg := agent.AgentMessage{
		Role:      "user",
		Content:   []agent.ContentBlock{agent.TextContent{Text: msg.Content}},
		Timestamp: time.Now().UnixMilli(),
	}

	// 5. 添加到会话
	sess.AddMessage(agentMsg)

	// 6. 获取历史消息
	historyMsg := sess.GetHistory(agentEntry.Config.MaxHistory)

	// 7. 调用 Agent 处理
	resp, err := agentEntry.Agent.Process(ctx, historyMsg)
	if err != nil {
		return fmt.Errorf("Agent处理失败: %w", err)
	}

	// 8. 添加回复到会话
	replyMsg := agent.AgentMessage{
		Role:      "assistant",
		Content:   []agent.ContentBlock{agent.TextContent{Text: resp}},
		Timestamp: time.Now().UnixMilli(),
	}
	sess.AddMessage(replyMsg)

	// 9. 发送响应
	outbound := &bus.OutBoundMessage{
		OutChannel: msg.InChannel,
		ChatID:     msg.ChatID,
		Content:    resp,
		ReplyTo:    msg.ID,
		TimeStamp:  time.Now(),
	}

	if err := g.msgBus.PublishOutBoundMessage(ctx, outbound); err != nil {
		return fmt.Errorf("发布响应失败: %w", err)
	}

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
	logger.L().Info().Msg("[Gateway] 执行健康检查")
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
