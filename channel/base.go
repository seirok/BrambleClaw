package channel

import (
	"brambleclaw/bus"
	"context"

	"github.com/google/uuid"
)

type BaseChannel interface {
	Name() string
	Start(context.Context) error
	Stop() error
	Send(*bus.OutBoundMessage) error
	IsAllowed(string) bool
}

type BaseChannelConfig struct {
	Enabled    bool     `json:"enabled"`
	AllowedIDs []string `json:"allowed_ids"`
}

type BaseChannelImpl struct {
	name   string
	config *BaseChannelConfig
	bus    *bus.MessageBus
}

func NewBaseChannelImpl(name string, config *BaseChannelConfig, bus *bus.MessageBus) *BaseChannelImpl {
	return &BaseChannelImpl{
		name:   name,
		config: config,
		bus:    bus,
	}
}

func BuildMediaScope(channel, chatID, messageID string) string {
	id := messageID
	if id == "" {
		id = uuid.NewString()
	}
	return channel + ":" + chatID + ":" + id
}

func (c *BaseChannelImpl) Name() string {
	return c.name
}
func (c *BaseChannelImpl) IsAllowed(id string) bool {
	// Channel 是否使能
	if c.config.Enabled == false {
		return false
	}
	// id是否在禁止列表中
	for _, aid := range c.config.AllowedIDs {
		if aid == id {
			return true
		}
	}
	return false
}

func (c *BaseChannelImpl) PublishInBoundMessage(ctx context.Context, in_msg *bus.InBoundMessage) error {
	in_msg.InChannel = c.Name()
	err := c.bus.PublishInBoundMessage(ctx, in_msg)
	if err != nil {
		return err
	}
	return nil
}
