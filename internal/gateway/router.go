package gateway

import (
	util "brambleclaw/internal"
	"brambleclaw/internal/agent"
	"brambleclaw/internal/bus"
	"brambleclaw/internal/config/structs"
	"brambleclaw/internal/interfaces"
	"context"
	"errors"
	"fmt"
)

type RouteResult struct {
	AgentName  string // 目标 Agent 名称
	SessionKey string // 会话标识符
	Channel    string // 通道名称
	ChatID     string // 聊天ID
	UserID     string // 用户ID
}

type Router struct {
	routes   []structs.GatewayRouteRule
	agentMgr *agent.AgentManager
}

func NewRouter(rules []structs.GatewayRouteRule, agentMgr *agent.AgentManager) *Router {
	return &Router{
		routes:   rules,
		agentMgr: agentMgr,
	}
}

func (r *Router) ResolveRoute(ctx context.Context, msg *bus.InBoundMessage) (*RouteResult, error) {
	if msg == nil {
		return nil, errors.New("inbound message is nil")
	}

	// 1. 确定初步的候选 Agent 名称
	// 逻辑：优先匹配渠道规则 -> 其次使用全局默认
	targetAgent := interfaces.DefaultAgentName
	if rule, found := r.getRouteForChannel(msg.InChannel); found {
		targetAgent = rule.Agent
	}

	// 2. 核心验证与降级逻辑 (Fallback)
	// 尝试获取目标 Agent，如果失败则尝试降级到默认 Agent
	_, err := r.agentMgr.Get(ctx, targetAgent)
	if err != nil {
		// 如果当前目标已经就是默认 Agent 了，说明默认的也没了，直接报错
		if targetAgent == interfaces.DefaultAgentName {
			return nil, fmt.Errorf("default agent %s unavailable: %w", targetAgent, err)
		}

		// 尝试降级：看一眼默认 Agent 在不在
		_, err = r.agentMgr.Get(ctx, interfaces.DefaultAgentName)
		if err != nil {
			return nil, fmt.Errorf("both target %s and default agent unavailable", targetAgent)
		}

		// 降级成功
		targetAgent = interfaces.DefaultAgentName
	}

	// 3. 构建并返回结果
	return &RouteResult{
		AgentName:  targetAgent,
		SessionKey: util.BuildSessionKey(targetAgent, msg.InChannel, msg.ChatID),
		Channel:    msg.InChannel,
		ChatID:     msg.ChatID,
		UserID:     msg.SenderID,
	}, nil
}

func (c *Router) getRouteForChannel(channel string) (*structs.GatewayRouteRule, bool) {
	var matched *structs.GatewayRouteRule
	maxPriority := -1

	for i := range c.routes {
		route := &c.routes[i]
		if route.Channel == channel && route.Priority > maxPriority {
			matched = route
			maxPriority = route.Priority
		}
	}
	return matched, matched != nil
}
