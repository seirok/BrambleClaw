package channel

import (
	"context"
	"fmt"
	"miniGoClaw/bus"
)

// CLIChannel CLI通道
type CLIChannel struct {
	*BaseChannelImpl
	running bool
}

// NewCLIChannel 创建CLI通道
func NewCLIChannel(config *BaseChannelConfig, bus *bus.MessageBus) *CLIChannel {
	return &CLIChannel{
		BaseChannelImpl: NewBaseChannelImpl("cli", config, bus),
		running:         false,
	}
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
func (c *CLIChannel) Stop() error {
	c.running = false
	return nil
}

// Send 发送消息
func (c *CLIChannel) Send(msg *bus.OutBoundMessage) error {
	fmt.Printf("\n> %s\n", msg.Content)
	return nil
}
