package channel

import (
	"context"
	"fmt"
	"neoclaw/internal/bus"
)

// CLIChannel CLI通道

// 编译期检查是否实现了接口
var _ BaseChannel = (*CLIChannel)(nil)

type CLIChannel struct {
	*BaseChannelImpl
	running    bool
	onResponse func(content, msgType string) // 回调：Agent 回复到达
}

// NewCLIChannel 创建CLI通道
func NewCLIChannel(config *BaseChannelConfig, bus *bus.MessageBus) *CLIChannel {
	return &CLIChannel{
		BaseChannelImpl: NewBaseChannelImpl("cli", config, bus),
		running:         false,
	}
}

// SetOnResponse 设置 Agent 回复回调（用于 TUI）
func (c *CLIChannel) SetOnResponse(f func(content, msgType string)) {
	c.onResponse = f
}

// Start 启动通道
func (c *CLIChannel) Start(ctx context.Context) error {
	if !c.config.Enabled {
		return nil
	}
	c.running = true
	return nil
}

// Stop 停止通道
func (c *CLIChannel) Stop(ctx context.Context) error {
	c.running = false
	return nil
}

// Send 发送消息
func (c *CLIChannel) Send(ctx context.Context, msg *bus.OutBoundMessage) error {
	if c.onResponse != nil {
		c.onResponse(msg.Content, msg.MsgType)
		return nil
	}
	// fallback: 无 TUI 时（非交互模式）仍用 fmt.Printf
	fmt.Printf("\n> %s\n", msg.Content)
	return nil
}
