package cli

import (
	"brambleclaw/internal/agent"
	"brambleclaw/internal/bus"
	"brambleclaw/internal/channel"
	"brambleclaw/internal/cli/tui"
	"brambleclaw/internal/config"
	"brambleclaw/internal/events"
	"brambleclaw/internal/gateway"
	"brambleclaw/internal/hook"
	"brambleclaw/internal/logger"
	"brambleclaw/internal/runtime"
	"brambleclaw/internal/teamtool"
	"brambleclaw/internal/tools"
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
)

var agentCmd = &cobra.Command{
	Use:   "agent",
	Short: "启动 AI 代理服务",
	RunE:  runAgent,
}

var (
	agentMessage string
	agentSession string
)

func init() {
	agentCmd.Flags().StringVarP(&agentMessage, "message", "m", "", "非交互式执行：发送一条消息后退出")
	agentCmd.Flags().StringVarP(&agentSession, "session", "s", "default", "指定 Session Key，保留上下文对话")

	rootCmd.AddCommand(agentCmd)
}

func runAgent(cmd *cobra.Command, args []string) error {
	logger.L().Debug().Msg("加载系统配置...")

	cfg := config.Get()
	if cfg == nil {
		return fmt.Errorf("系统配置加载失败")
	}

	logger.Setup(cfg.Log.Level, cfg.Log.ConsoleEnabled)
	logger.L().Debug().Msg("日志配置已加载")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 1. 初始化消息总线
	logger.L().Debug().Msg("初始化消息总线...")
	msgBus := bus.NewMessageBus(cfg.BusBufSize)

	// 2. 初始化通道管理器
	logger.L().Debug().Msg("初始化通道管理器...")
	channelManager := channel.NewChannelManager(msgBus)
	logger.L().Debug().Msg("Initializing channel manager with config...")
	if err := channelManager.Initialize(ctx, cfg); err != nil {
		return fmt.Errorf("failed to initialize channel manager: %w", err)
	}

	// 5. 初始化 AgentManager
	logger.L().Debug().Msg("初始化 AgentManager...")
	agentManager := agent.NewAgentManager(msgBus, runtime.NewAgentRuntime())
	agentManager.SetToolFactory(func(a *agent.Agent) []tools.Tool {
		return []tools.Tool{teamtool.NewCreateTeamTool(a)}
	})

	// 6. 初始化 Gateway
	logger.L().Debug().Msg("初始化 Gateway...")
	gwCfg := config.Get().Gateway
	gw := gateway.NewGateway(
		gateway.WithRouter(gateway.NewRouter(gwCfg.Routes, agentManager)),
		gateway.WithAgentManager(agentManager),
		gateway.WithChannelManager(channelManager),
		gateway.WithMessageBus(msgBus),
	)

	// 7. 启动 Gateway 和 Channel
	logger.L().Debug().Msg("启动 Gateway 和 Channel...")
	if err := gw.Start(ctx); err != nil {
		return err
	}

	if err := channelManager.StartAll(ctx); err != nil {
		return err
	}

	// 等待 Gateway 准备好
	time.Sleep(100 * time.Millisecond)

	// 设置信号处理
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// 交互式与非交互式执行
	if agentMessage != "" {
		// 非交互式执行
		inboundMsg := &bus.InBoundMessage{
			InChannel: "cli",
			SenderID:  "cli",
			ChatID:    agentSession,
			Content:   agentMessage,
			TimeStamp: time.Now(),
		}

		// 订阅接收返回的消息
		sub := msgBus.Subscribe()
		defer msgBus.Unsubscribe(sub.ID)

		if err := msgBus.PublishInBoundMessage(ctx, inboundMsg); err != nil {
			return fmt.Errorf("发送消息失败: %w", err)
		}

		select {
		case outMsg := <-sub.Channel:
			if outMsg.ReplyTo == inboundMsg.ID {
				time.Sleep(100 * time.Millisecond)
			}
		case <-time.After(30 * time.Second):
			return fmt.Errorf("执行超时")
		case <-ctx.Done():
		case <-sigChan:
			fmt.Println("\n收到退出信号...")
		}
	} else {
		// ========== 交互式执行 ==========

		// 创建 EventBus 并注册 observers
		tvCfg := cfg.Hooks.ThinkingVisibility
		var eventBus *events.EventBus
		if tvCfg.Enabled {
			eventBus = events.NewEventBus(tvCfg.MaxEvents)
			hook.RegisterObservers(eventBus, tvCfg)
		}

		model := tui.NewAppModel(msgBus, agentSession)
		p := tea.NewProgram(model, tea.WithAltScreen())

		// 获取 CLIChannel 并设置回调
		var responseChan chan string = make(chan string, 1)
		cliChan, err := channelManager.Get(ctx, "cli")
		if err == nil {
			if c, ok := cliChan.(*channel.CLIChannel); ok {
				c.SetOnResponse(func(content string) {
					responseChan <- content
				})
			}
		}

		// 响应通道 -> TUI
		go func() {
			for content := range responseChan {
				p.Send(tui.AgentResponseMsg{Content: content})
			}
		}()

		// EventBus -> TUI
		if eventBus != nil {
			go func() {
				for evt := range eventBus.Subscribe() {
					p.Send(tui.ThinkingEventMsg{Event: evt})
				}
			}()
			defer eventBus.Close()
		}

		if _, err := p.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "TUI 错误: %v\n", err)
		}

		close(responseChan)
	}

	// 停止
	fmt.Println("Shutting down... Hope to see you next time!😊")
	stopCtx, stopCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer stopCancel()
	if err := gw.Stop(stopCtx); err != nil {
		logger.L().Error().Err(err).Msg("停止 Gateway 失败")
	}
	if err := channelManager.StopAll(ctx); err != nil {
		logger.L().Error().Err(err).Msg("停止 ChannelManager 失败")
	}
	if err := agentManager.StopAll(ctx); err != nil {
		logger.L().Error().Err(err).Msg("停止 AgentManager 失败")
	}
	os.Exit(0)
	return nil
}
