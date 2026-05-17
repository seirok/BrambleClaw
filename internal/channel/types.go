package channel

import (
	"context"
	"errors"
	"fmt"
	"neoclaw/internal/bus"
	"neoclaw/internal/interfaces"
	"neoclaw/internal/logger"
	"neoclaw/internal/registry"
)

var (
	ErrChannelNotFound = errors.New("channel not found")
	ErrChannelExists   = errors.New("channel already exists")
)

var _ interfaces.Registry[BaseChannel] = (*ChannelRegistry)(nil)

type ChannelRegistry struct {
	*registry.GenericRegistry[BaseChannel]
}

func NewChannelRegistry() *ChannelRegistry {
	return &ChannelRegistry{
		GenericRegistry: registry.NewGenericRegistry[BaseChannel](
			func(name string) error { return fmt.Errorf("%w: %s", ErrChannelExists, name) },
			func(name string) error { return fmt.Errorf("%w: %s", ErrChannelNotFound, name) },
			nil,
		),
	}
}

func (r *ChannelRegistry) Register(ctx context.Context, name string, value BaseChannel) error {
	if err := r.GenericRegistry.Register(ctx, name, value); err != nil {
		return err
	}
	logger.L().Debug().Str("channel", name).Msg("channel registered")
	return nil
}

func (r *ChannelRegistry) Unregister(ctx context.Context, name string) error {
	if err := r.GenericRegistry.Unregister(ctx, name); err != nil {
		return err
	}
	logger.L().Debug().Str("channel", name).Msg("channel unregistered")
	return nil
}

type BaseChannel interface {
	Name() string
	Start(context.Context) error
	Stop(context.Context) error
	Send(context.Context, *bus.OutBoundMessage) error
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
