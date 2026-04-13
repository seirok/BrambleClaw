package gateway

import (
	"fmt"
	"strings"

	"miniGoClaw/bus"
)

// RouteResult 路由解析结果
type RouteResult struct {
	AgentName  string // 目标 Agent 名称
	SessionKey string // 会话标识符
	Channel    string // 通道名称
	ChatID     string // 聊天ID
	UserID     string // 用户ID
}

// Router 消息路由器
// 负责解析入站消息，确定消息应该路由到哪个 Agent
type Router struct {
	config   *GatewayConfig
	registry *AgentRegistry
}

// NewRouter 创建新的路由器
func NewRouter(config *GatewayConfig, registry *AgentRegistry) *Router {
	return &Router{
		config:   config,
		registry: registry,
	}
}

// ResolveRoute 解析消息路由
// 根据入站消息和配置确定目标 Agent 和会话标识符
func (r *Router) ResolveRoute(msg *bus.InBoundMessage) (*RouteResult, error) {
	if msg == nil {
		return nil, fmt.Errorf("消息不能为空")
	}

	channel := msg.InChannel
	chatID := msg.ChatID
	userID := msg.SenderID

	// 构建基础结果
	result := &RouteResult{
		Channel: channel,
		ChatID:  chatID,
		UserID:  userID,
	}

	// 1. 查找匹配的路由规则
	routeRule, found := r.config.GetRouteForChannel(channel)
	if found {
		result.AgentName = routeRule.Agent
	} else {
		// 2. 使用默认 Agent
		result.AgentName = r.config.DefaultAgent
	}

	// 3. 验证 Agent 是否存在
	_, exists := r.registry.GetAgent(result.AgentName)
	if !exists {
		// 如果指定的 Agent 不存在，尝试使用默认 Agent
		if result.AgentName != r.config.DefaultAgent {
			_, exists = r.registry.GetAgent(r.config.DefaultAgent)
			if exists {
				result.AgentName = r.config.DefaultAgent
			} else {
				return nil, fmt.Errorf("Agent不存在(name=%s, default=%s)", result.AgentName, r.config.DefaultAgent)
			}
		} else {
			return nil, fmt.Errorf("默认Agent不存在(name=%s)", r.config.DefaultAgent)
		}
	}

	// 4. 构建会话标识符
	// 格式: agent:{agent_name}:{channel}:{chat_type}:{chat_id}
	// 示例: agent:main:weixin:direct:wxid_abc123
	result.SessionKey = BuildSessionKey(result.AgentName, channel, "direct", chatID)

	return result, nil
}

// BuildSessionKey 构建会话标识符
// 格式: agent:{agent_name}:{channel}:{chat_type}:{chat_id}
func BuildSessionKey(agentName, channel, chatType, chatID string) string {
	return fmt.Sprintf("agent:%s:%s:%s:%s", agentName, channel, chatType, chatID)
}

// ParseSessionKey 解析会话标识符
// 将 session key 解析为各个组成部分
func ParseSessionKey(sessionKey string) (agentName, channel, chatType, chatID string, err error) {
	parts := strings.Split(sessionKey, ":")
	if len(parts) != 5 {
		return "", "", "", "", fmt.Errorf("无效的sessionKey格式(期望5部分,实际%d部分)", len(parts))
	}

	if parts[0] != "agent" {
		return "", "", "", "", fmt.Errorf("无效的sessionKey前缀(期望'agent',实际'%s')", parts[0])
	}

	return parts[1], parts[2], parts[3], parts[4], nil
}

// MatchRouteCondition 检查消息是否匹配路由条件
// 根据消息内容和条件进行匹配判断
func MatchRouteCondition(msg *bus.InBoundMessage, conditions map[string]string) bool {
	for key, expectedValue := range conditions {
		var actualValue string

		switch key {
		case "user_id", "sender_id":
			actualValue = msg.SenderID
		case "channel":
			actualValue = msg.InChannel
		case "chat_id":
			actualValue = msg.ChatID
		default:
			// 未知条件，不匹配
			return false
		}

		// 精确匹配
		if actualValue != expectedValue {
			return false
		}
	}

	// 所有条件都匹配
	return true
}
