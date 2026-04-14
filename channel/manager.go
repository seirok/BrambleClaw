package channel

import (
	"brambleclaw/bus"
	"brambleclaw/logger"
	"context"
	"fmt"
	"sync"
)

// Manager 通道管理器
type Manager struct {
	channels map[string]BaseChannel
	msgBus   *bus.MessageBus
	mu       sync.RWMutex
}

// NewManager 创建通道管理器
func NewManager(bus *bus.MessageBus) *Manager {
	return &Manager{
		channels: make(map[string]BaseChannel),
		msgBus:   bus,
		mu:       sync.RWMutex{},
	}
}

// Register 注册通道
func (m *Manager) Register(channel BaseChannel) error {
	// 检查Channel是否被注册
	_, ok := m.channels[channel.Name()]
	if ok {
		return fmt.Errorf("channel %s already exists", channel.Name())
	}

	m.channels[channel.Name()] = channel
	logger.L().Debug().Str("channel", channel.Name()).Msg("channel registered")

	return nil
}

// Start 启动所有通道
func (m *Manager) Start(ctx context.Context) error {
	if len(m.channels) == 0 {
		logger.L().Info().Msg("no channels registered")
		return nil
	}
	for _, channel := range m.channels {
		err := channel.Start(ctx)
		if err != nil {
			logger.L().Error().Err(err).Str("channel", channel.Name()).Msg("channel start error")
			continue
		}
	}

	return nil
}

// Stop 停止所有通道
func (m *Manager) Stop() error {
	if len(m.channels) == 0 {
		logger.L().Info().Msg("no channels registered")
		return nil
	}
	for _, channel := range m.channels {
		err := channel.Stop()
		if err != nil {
			logger.L().Error().Err(err).Str("channel", channel.Name()).Msg("channel stop error")
			return err
		}
	}
	return nil
}

// Get 获取通道
func (m *Manager) Get(name string) (BaseChannel, bool) {
	channel, ok := m.channels[name]
	if !ok {
		logger.L().Error().Str("channel", name).Msg("channel not found")
		return nil, false
	}

	return channel, true
}

// DispatchOutbound 分发出站消息
func (m *Manager) DispatchOutbound(ctx context.Context) error {
	logger.L().Info().Msg("start dispatch outbound")
	sub := m.msgBus.Subscribe()
	for msg := range sub.Channel {
		channel, ok := m.Get(msg.OutChannel)
		if !ok {
			logger.L().Error().Str("channel", msg.OutChannel).Msg("channel not found")
			continue
		}
		if err := channel.Send(msg); err != nil {
			logger.L().Error().Err(err).Str("channel", msg.OutChannel).Msg("send message error")
		}
	}
	return nil
}
