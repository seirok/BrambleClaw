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

	// Runtime state
	mu      sync.RWMutex
	running bool
	wg      sync.WaitGroup
	cancel  context.CancelFunc
}

// Option is function type for configuring Gateway
type Option func(*Gateway)

// NewGateway creates new Gateway instance, using Functional Options pattern
func NewGateway(opts ...Option) *Gateway {
	g := &Gateway{
		running: false,
	}

	// Apply all options
	for _, opt := range opts {
		opt(g)
	}

	if g.agentRuntime == nil {
		g.agentRuntime = runtime.NewAgentRuntime()
	}

	return g
}

// WithRouter sets router
func WithRouter(router *Router) Option {
	return func(g *Gateway) {
		g.router = router
	}
}

// WithAgentManager sets Agent manager
func WithAgentManager(am interfaces.Manager[*agent.Agent]) Option {
	return func(g *Gateway) {
		g.agentManager = am
	}
}

// WithChannelManager sets channel manager
func WithChannelManager(cm interfaces.Manager[channel.BaseChannel]) Option {
	return func(g *Gateway) {
		g.channelManager = cm
	}
}

// WithMessageBus sets message bus
func WithMessageBus(msgBus *bus.MessageBus) Option {
	return func(g *Gateway) {
		g.msgBus = msgBus
	}
}

// WithAgentRuntime sets Agent runtime
func WithAgentRuntime(rt *runtime.AgentRuntime) Option {
	return func(g *Gateway) {
		g.agentRuntime = rt
	}
}

func (g *Gateway) Name() string {
	return "Gateway service"
}

// Start starts Gateway
func (g *Gateway) Start(ctx context.Context) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.running {
		return fmt.Errorf("Gateway is already running")
	}

	ctx, g.cancel = context.WithCancel(ctx)
	g.running = true

	// Start Agent management
	err := g.agentManager.Initialize(ctx, config.Get())
	if err != nil {
		return err
	}

	err = g.agentManager.StartAll(ctx)
	if err != nil {
		return err
	}

	// Start message processing loop
	g.wg.Add(1)
	go g.processMessageLoop(ctx)

	// Start outbound message dispatch
	g.wg.Add(1)
	go g.dispatchOutboundLoop(ctx)

	// Start health check

	g.wg.Add(1)
	go g.healthCheckLoop(ctx)

	logger.L().Info().Str("component", "Gateway").Msg("Started")
	// Trigger Gateway start hook (fire-and-forget)
	hook.Emit(ctx, "hook.point.gateway.start", g)
	return nil
}

// Stop stops Gateway, closes components in sequence
func (g *Gateway) Stop(ctx context.Context) error {
	g.mu.Lock()
	if !g.running {
		g.mu.Unlock()
		return nil
	}
	g.running = false
	g.mu.Unlock()

	// 1. Notify all goroutines created in Start to exit
	g.cancel()

	//  Wait for goroutines to finish
	done := make(chan struct{})
	go func() {
		g.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-ctx.Done():
		logger.L().Warn().Str("component", "Gateway").Msg("goroutine stop timeout, continue closing components")
	}

	// 3. Close channel manager (stop all outbound channels)
	if err := g.channelManager.StopAll(ctx); err != nil {
		logger.L().Error().Err(err).Str("component", "Gateway").Msg("Failed to close channel manager")
	}

	// 4. Close Agent manager (stop all Agents, Sessions, MCP)
	if err := g.agentManager.StopAll(ctx); err != nil {
		logger.L().Error().Err(err).Str("component", "Gateway").Msg("Failed to close Agent manager")
	}

	logger.L().Info().Str("component", "Gateway").Msg("Stopped")
	return nil
}

// processMessageLoop main message processing loop
func (g *Gateway) processMessageLoop(ctx context.Context) {
	defer g.wg.Done()

	logger.L().Debug().Str("component", "Gateway").Msg("Message processing loop started")
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		// Consume inbound message
		msg, err := g.msgBus.ConsumeInBoundMessage(ctx) // blocking wait
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			logger.L().Error().Err(err).Str("component", "Gateway").Msg("Failed to consume message, context may have been canceled")
			continue
		}

		// Process message
		if err := g.handleMessage(ctx, msg); err != nil {
			logger.L().Error().Err(err).Str("component", "Gateway").Msg("Failed to process message")
		}
	}
}

// handleMessage processes single message
func (g *Gateway) handleMessage(ctx context.Context, msg *bus.InBoundMessage) error {
	//  Resolve route
	route, err := g.router.ResolveRoute(ctx, msg)
	if err != nil {
		return fmt.Errorf("failed to resolve route: %w", err)
	}

	// Trigger route hook
	hook.Emit(ctx, "hook.point.message.route", route)

	//  Get Agent
	agent_, err := g.agentManager.Get(ctx, route.AgentName)
	if err != nil {
		return err
	}

	// Agent handles message
	agent_.HandleMessage(ctx, msg)
	return nil
}

// dispatchOutboundLoop outbound message dispatch loop
func (g *Gateway) dispatchOutboundLoop(ctx context.Context) {
	defer g.wg.Done()

	// Start subscription message dispatch
	logger.L().Debug().Str("component", "Gateway").Msg("Subscription dispatch started")
	go g.msgBus.DistributeOutBoundMessage(ctx)

	logger.L().Debug().Str("component", "Gateway").Msg("Response dispatch started")
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

			// Send message through Channel Manager
			ch, err := g.channelManager.Get(ctx, msg.OutChannel)
			if err != nil {
				logger.L().Error().Err(err).Str("component", "Gateway").Str("channel", msg.OutChannel).Msg("Channel does not exist")
				continue
			}

			if err := ch.Send(ctx, msg); err != nil {
				logger.L().Error().Err(err).Str("component", "Gateway").Str("channel", msg.OutChannel).Msg("Failed to send message")
			}
		}
	}
}

// healthCheckLoop health check loop
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

// performHealthCheck performs health check
func (g *Gateway) performHealthCheck() {
	// Check all channel status
	// Specific health check logic can be implemented here
}

// IsRunning checks if Gateway is running
func (g *Gateway) IsRunning() bool {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.running
}
