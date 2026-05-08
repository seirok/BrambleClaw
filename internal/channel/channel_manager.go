package channel

import (
	"brambleclaw/internal/bus"
	"brambleclaw/internal/config"
	"brambleclaw/internal/interfaces"
	"brambleclaw/internal/logger"
	"context"
	"fmt"
	"sync"
)

type ChannelManager struct {
	channelRegistry interfaces.Registry[BaseChannel]
	msgBus          *bus.MessageBus
	mu              sync.RWMutex
	status          interfaces.ManagerStatus
}

// NewChannelManager 创建通道管理器
func NewChannelManager(bus *bus.MessageBus) *ChannelManager {
	return &ChannelManager{
		channelRegistry: NewChannelRegistry(),
		msgBus:          bus,
		status:          interfaces.StatusIdle,
		mu:              sync.RWMutex{},
	}
}

// Initialize 初始化管理器（Service 接口）
func (m *ChannelManager) Initialize(ctx context.Context, cfg any) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	logger.L().Debug().Msg("Starting ChannelManager initialization")

	// 将 cfg 转换为 *config.Config 类型
	cfgObj, ok := cfg.(*config.Config)
	if !ok {
		return fmt.Errorf("invalid config type: expected *config.Config, got %T", cfg)
	}

	logger.L().Debug().
		Bool("cli_enabled", cfgObj.Channels.CLI.Enabled).
		Bool("dingtalk_enabled", cfgObj.Channels.DingTalk.Enabled).
		Bool("feishu_enabled", cfgObj.Channels.Feishu.Enabled).
		Msg("Channel configuration")

	// 初始化 CLI 通道
	cliConfig := &BaseChannelConfig{
		Enabled:    cfgObj.Channels.CLI.Enabled,
		AllowedIDs: cfgObj.Channels.CLI.AllowedIDs,
	}
	cliChannel := NewCLIChannel(cliConfig, m.msgBus)

	if err := m.channelRegistry.Register(ctx, "cli", cliChannel); err != nil {
		logger.L().Error().Err(err).Msg("Failed to register CLI channel")
		return fmt.Errorf("failed to register CLI channel: %w", err)
	}
	logger.L().Debug().Bool("enabled", cfgObj.Channels.CLI.Enabled).Strs("allowed_ids", cfgObj.Channels.CLI.AllowedIDs).Msg("CLI channel registered")

	// 初始化 DingTalk 通道
	dingtalkCfg := cfgObj.Channels.DingTalk
	dingtalkChannel, err := NewDingTalkChannel(dingtalkCfg, m.msgBus)
	if err != nil {
		return err
	}
	if err = m.channelRegistry.Register(ctx, "dingtalk", dingtalkChannel); err != nil {
		logger.L().Error().Err(err).Msg("Failed to register DingTalk channel")
		return fmt.Errorf("failed to register DingTalk channel: %w", err)
	}

	// 初始化 Feishu 通道
	feishuCfg := cfgObj.Channels.Feishu
	feishuChannel, err := NewFeishuChannel(feishuCfg, m.msgBus)
	if err != nil {
		return err
	}
	if err = m.channelRegistry.Register(ctx, "feishu", feishuChannel); err != nil {
		logger.L().Error().Err(err).Msg("Failed to register Feishu channel")
		return fmt.Errorf("failed to register Feishu channel: %w", err)
	}

	m.status = interfaces.StatusRunning
	logger.L().Debug().Int("registered_channels", len(m.channelRegistry.List(ctx))).Msg("ChannelManager initialization completed")

	return nil
}

// StartAll 启动所有通道（Manager 接口）
func (m *ChannelManager) StartAll(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	channels := m.channelRegistry.List(ctx)
	if len(channels) == 0 {
		logger.L().Info().Msg("no channels registered")
		m.status = interfaces.StatusRunning
		return nil
	}

	var errs []error
	for _, channel := range channels {
		err := channel.Start(ctx)
		if err != nil {
			logger.L().Error().Err(err).Str("channel", channel.Name()).Msg("channel start error")
			errs = append(errs, fmt.Errorf("channel %q start failed: %w", channel.Name(), err))
		}
	}

	m.status = interfaces.StatusRunning
	if len(errs) > 0 {
		return fmt.Errorf("channels start errors: %v", errs)
	}
	return nil
}

// StopAll 停止所有通道（Manager 接口）
func (m *ChannelManager) StopAll(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	channels := m.channelRegistry.List(ctx)
	if len(channels) == 0 {
		logger.L().Info().Msg("no channels registered")
		m.status = interfaces.StatusStopped
		return nil
	}

	var errs []error
	for _, channel := range channels {
		err := channel.Stop(ctx)
		if err != nil {
			logger.L().Error().Err(err).Str("channel", channel.Name()).Msg("channel stop error")
			errs = append(errs, fmt.Errorf("channel %q stop failed: %w", channel.Name(), err))
		}
	}

	m.status = interfaces.StatusStopped
	if len(errs) > 0 {
		return fmt.Errorf("channels stop errors: %v", errs)
	}
	return nil
}

// Add 添加一个通道（Manager 接口）
func (m *ChannelManager) Add(ctx context.Context, id string, item BaseChannel) error {
	return m.channelRegistry.Register(ctx, id, item)
}

// Remove 移除一个通道（Manager 接口）
func (m *ChannelManager) Remove(ctx context.Context, id string) error {
	return m.channelRegistry.Unregister(ctx, id)
}

// Get 获取一个通道（Manager 接口）
func (m *ChannelManager) Get(ctx context.Context, id string) (BaseChannel, error) {
	return m.channelRegistry.Get(ctx, id)
}

// List 返回所有通道列表（Manager 接口）
func (m *ChannelManager) List(ctx context.Context) []BaseChannel {
	return m.channelRegistry.List(ctx)
}

// Status 返回管理器状态（Manager 接口）
func (m *ChannelManager) Status() interfaces.ManagerStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.status
}

// DispatchOutbound 分发出站消息
func (m *ChannelManager) DispatchOutbound(ctx context.Context) error {
	logger.L().Info().Msg("start dispatch outbound")
	sub := m.msgBus.Subscribe()
	for msg := range sub.Channel {
		channel, err := m.Get(ctx, msg.OutChannel)
		if err != nil {
			logger.L().Error().Err(err).Str("channel", msg.OutChannel).Msg("channel not found")
			continue
		}
		if err := channel.Send(ctx, msg); err != nil {
			logger.L().Error().Err(err).Str("channel", msg.OutChannel).Msg("send message error")
		}
	}
	return nil
}
