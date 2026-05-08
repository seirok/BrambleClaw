package gateway

import (
	"brambleclaw/internal/agent"
	"brambleclaw/internal/bus"
	"brambleclaw/internal/channel"
	"brambleclaw/internal/config"
	"brambleclaw/internal/hook"
	"brambleclaw/internal/interfaces"
	"brambleclaw/internal/logger"
	"brambleclaw/internal/runtime"
	"context"
	"fmt"
	"sync"
	"time"
)

type Gateway struct {
	router         *Router
	agentManager   interfaces.Manager[*agent.Agent]
	channelManager interfaces.Manager[channel.BaseChannel]
	msgBus         *bus.MessageBus
	agentRuntime   *runtime.AgentRuntime

	// 运行时状态
	mu      sync.RWMutex
	running bool
	wg      sync.WaitGroup
	cancel  context.CancelFunc
}

// Option 是用于配置 Gateway 的函数类型
type Option func(*Gateway)

// NewGateway 创建新的 Gateway 实例，使用 Functional Options 模式
func NewGateway(opts ...Option) *Gateway {
	g := &Gateway{
		running: false,
	}

	// 应用所有选项
	for _, opt := range opts {
		opt(g)
	}

	if g.agentRuntime == nil {
		g.agentRuntime = runtime.NewAgentRuntime()
	}

	return g
}

// WithRouter 设置路由
func WithRouter(router *Router) Option {
	return func(g *Gateway) {
		g.router = router
	}
}

// WithAgentManager 设置 Agent 管理器
func WithAgentManager(am interfaces.Manager[*agent.Agent]) Option {
	return func(g *Gateway) {
		g.agentManager = am
	}
}

// WithChannelManager 设置通道管理器
func WithChannelManager(cm interfaces.Manager[channel.BaseChannel]) Option {
	return func(g *Gateway) {
		g.channelManager = cm
	}
}

// WithMessageBus 设置消息总线
func WithMessageBus(msgBus *bus.MessageBus) Option {
	return func(g *Gateway) {
		g.msgBus = msgBus
	}
}

// WithAgentRuntime 设置 Agent 运行时
func WithAgentRuntime(rt *runtime.AgentRuntime) Option {
	return func(g *Gateway) {
		g.agentRuntime = rt
	}
}

func (g *Gateway) Name() string {
	return "Gateway service"
}

// Start 启动 Gateway
func (g *Gateway) Start(ctx context.Context) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.running {
		return fmt.Errorf("Gateway 已处于运行状态")
	}

	ctx, g.cancel = context.WithCancel(ctx)
	g.running = true
	// 启动 Agent 管理
	err := g.agentManager.Initialize(ctx, config.Get())
	if err != nil {
		return err
	}
	err = g.agentManager.StartAll(ctx)
	if err != nil {
		return err
	}

	// 启动消息处理循环
	g.wg.Add(1)
	go g.processMessageLoop(ctx)

	// 启动出站消息分发
	g.wg.Add(1)
	go g.dispatchOutboundLoop(ctx)

	// 启动健康检查

	g.wg.Add(1)
	go g.healthCheckLoop(ctx)

	logger.L().Info().Str("component", "Gateway").Msg("已启动")
	// 触发 Gateway 启动钩子（fire-and-forget）
	hook.Emit(ctx, "hook.point.gateway.start", g)
	return nil
}

// Stop 停止 Gateway，依次关闭各组件
func (g *Gateway) Stop(ctx context.Context) error {
	g.mu.Lock()
	if !g.running {
		g.mu.Unlock()
		return nil
	}
	g.running = false
	g.mu.Unlock()

	// 1. 通知 Start 中创建的所有 goroutine 退出
	g.cancel()

	//  等待goroutine 结束
	done := make(chan struct{})
	go func() {
		g.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-ctx.Done():
		logger.L().Warn().Str("component", "Gateway").Msg("goroutine 停止超时，继续关闭组件")
	}

	// 3. 关闭通道管理器（停止所有出站通道）
	if err := g.channelManager.StopAll(ctx); err != nil {
		logger.L().Error().Err(err).Str("component", "Gateway").Msg("关闭通道管理器失败")
	}

	// 4. 关闭 Agent 管理器（停止所有 Agent、Session、MCP）
	if err := g.agentManager.StopAll(ctx); err != nil {
		logger.L().Error().Err(err).Str("component", "Gateway").Msg("关闭 Agent 管理器失败")
	}

	logger.L().Info().Str("component", "Gateway").Msg("已停止")
	return nil
}

// processMessageLoop 消息处理主循环
func (g *Gateway) processMessageLoop(ctx context.Context) {
	defer g.wg.Done()

	logger.L().Debug().Str("component", "Gateway").Msg("消息处理循环启动")
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		// 消费入站消息
		msg, err := g.msgBus.ConsumeInBoundMessage(ctx) // 阻塞等待
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			logger.L().Error().Err(err).Str("component", "Gateway").Msg("消费消息失败，context 可能已取消")
			continue
		}

		// 处理消息
		if err := g.handleMessage(ctx, msg); err != nil {
			logger.L().Error().Err(err).Str("component", "Gateway").Msg("处理消息失败")
		}
	}
}

// handleMessage 处理单条消息
func (g *Gateway) handleMessage(ctx context.Context, msg *bus.InBoundMessage) error {
	//  解析路由
	route, err := g.router.ResolveRoute(ctx, msg)
	if err != nil {
		return fmt.Errorf("路由解析失败: %w", err)
	}

	// 触发路由钩子
	hook.Emit(ctx, "hook.point.message.route", route)

	//  获取 Agent
	agent_, err := g.agentManager.Get(ctx, route.AgentName)
	if err != nil {
		return err
	}

	// Agent 处理消息
	agent_.HandleMessage(ctx, msg)
	return nil
}

// dispatchOutboundLoop 出站消息分发循环
func (g *Gateway) dispatchOutboundLoop(ctx context.Context) {
	defer g.wg.Done()

	// 启动订阅消息分发
	logger.L().Debug().Str("component", "Gateway").Msg("订阅分发启动")
	go g.msgBus.DistributeOutBoundMessage(ctx)

	logger.L().Debug().Str("component", "Gateway").Msg("响应分发启动")
	sub := g.msgBus.Subscribe()
	defer g.msgBus.Unsubscribe(sub.ID)

	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-sub.Channel:
			if !ok {
				return
			}

			// 通过 Channel Manager 发送消息
			ch, err := g.channelManager.Get(ctx, msg.OutChannel)
			if err != nil {
				logger.L().Error().Err(err).Str("component", "Gateway").Str("channel", msg.OutChannel).Msg("通道不存在")
				continue
			}

			if err := ch.Send(ctx, msg); err != nil {
				logger.L().Error().Err(err).Str("component", "Gateway").Str("channel", msg.OutChannel).Msg("发送消息失败")
			}
		}
	}
}

// healthCheckLoop 健康检查循环
func (g *Gateway) healthCheckLoop(ctx context.Context) {
	defer g.wg.Done()

	ticker := time.NewTicker(time.Duration(60) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
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
}

// IsRunning 检查 Gateway 是否正在运行
func (g *Gateway) IsRunning() bool {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.running
}
