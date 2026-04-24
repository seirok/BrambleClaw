package channel

import (
	"brambleclaw/internal/bus"
	"brambleclaw/internal/interfaces"
	"brambleclaw/internal/logger"
	"context"
	"errors"
	"fmt"
	"sync"
)

var (
	ErrChannelNotFound = errors.New("channel not found")
	ErrChannelExists   = errors.New("channel already exists")
)

var _ interfaces.Registry[BaseChannel] = (*ChannelRegistry)(nil)

type ChannelRegistry struct {
	channels map[string]BaseChannel
	mu       sync.RWMutex
}

func NewChannelRegistry() *ChannelRegistry {
	return &ChannelRegistry{
		channels: make(map[string]BaseChannel),
		mu:       sync.RWMutex{},
	}
}

func (r *ChannelRegistry) Register(ctx context.Context, name string, value BaseChannel) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.channels[name]; exists {
		return fmt.Errorf("%w: %s", ErrChannelExists, name)
	}

	r.channels[name] = value
	logger.L().Debug().Str("channel", name).Msg("channel registered")
	return nil
}

func (r *ChannelRegistry) Unregister(ctx context.Context, name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.channels[name]; !exists {
		return fmt.Errorf("%w: %s", ErrChannelNotFound, name)
	}

	delete(r.channels, name)
	logger.L().Debug().Str("channel", name).Msg("channel unregistered")
	return nil
}

func (r *ChannelRegistry) Get(ctx context.Context, name string) (BaseChannel, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ch, ok := r.channels[name]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrChannelNotFound, name)
	}
	return ch, nil
}

func (r *ChannelRegistry) List(ctx context.Context) []BaseChannel {
	r.mu.RLock()
	defer r.mu.RUnlock()

	list := make([]BaseChannel, 0, len(r.channels))
	for _, ch := range r.channels {
		list = append(list, ch)
	}
	return list
}

type BaseChannel interface {
	Name() string
	Start(context.Context) error
	Stop(context.Context) error
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
