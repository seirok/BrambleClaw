package cli

import (
	"context"
	"fmt"
	util "neoclaw/internal"
	"neoclaw/internal/agent"
	"neoclaw/internal/bus"
	"neoclaw/internal/channel"
	"neoclaw/internal/cli/tui"
	"neoclaw/internal/config"
	"neoclaw/internal/cron"
	"neoclaw/internal/events"
	"neoclaw/internal/gateway"
	"neoclaw/internal/hook"
	"neoclaw/internal/httpserver"
	"neoclaw/internal/interfaces"
	"neoclaw/internal/logger"
	"neoclaw/internal/runtime"
	"neoclaw/internal/skill"
	"neoclaw/internal/teamtool"
	"os"
	"os/signal"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
)

// responsePayload carries response content and message type from CLIChannel to TUI
type responsePayload struct {
	Content string
	MsgType string
}

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
	agentCmd.Flags().StringVarP(&agentMessage, "message", "m", "", "Non-interactive execution: send one message and exit")
	//	agentCmd.Flags().StringVarP(&agentSession, "session", "s", "default", "Specify Session Key, keep context conversation")

	rootCmd.AddCommand(agentCmd)
}

func runAgent(cmd *cobra.Command, args []string) error {
	logger.L().Debug().Msg("Loading system configuration...")

	cfg := config.Get()
	if cfg == nil {
		return fmt.Errorf("系统配置加载失败")
	}

	logger.Setup(cfg.Log.Level, cfg.Log.ConsoleEnabled)
	logger.L().Debug().Msg("Log configuration loaded")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 1. 初始化消息总线
	logger.L().Debug().Msg("Initializing message bus...")
	msgBus := bus.NewMessageBus(cfg.BusBufSize)

	// 2. 初始化通道管理器
	logger.L().Debug().Msg("Initializing ChannelManager...")
	channelManager := channel.NewChannelManager(msgBus)
	logger.L().Debug().Msg("Initializing channel manager with config...")
	if err := channelManager.Initialize(ctx, cfg); err != nil {
		return fmt.Errorf("failed to initialize channel manager: %w", err)
	}

	// 3. 初始化 Cron
	logger.L().Debug().Msg("Initializing CronService...")
	cronDataDir := cfg.Tools.Cron.DataDir
	var cronService *cron.CronService
	var cronTool *cron.CronTool
	if cfg.Tools.Cron.Enabled {
		cronService, cronTool = cron.NewCronServiceAndTool(msgBus, cronDataDir)
	}

	// 5. 初始化 SkillManager 和 AgentManager
	logger.L().Debug().Msg("Initializing SkillManager...")
	skillManager := skill.NewSkillManager(&cfg.Skill)

	logger.L().Debug().Msg("Initializing AgentManager...")
	agentManager := agent.NewAgentManager(msgBus, runtime.NewAgentRuntime())
	agentManager.SetSkillManager(skillManager)

	agentManager.SetToolFactory(func(a *agent.Agent) []interfaces.Tool {
		baseTools := []interfaces.Tool{teamtool.NewCreateTeamTool(a)}
		if cfg.Tools.Cron.Enabled && cronTool != nil {
			baseTools = append(baseTools, cronTool)
		}
		// Add activate_skill tool
		skTool := skill.NewActivateSkillTool(skillManager)
		baseTools = append(baseTools, skTool)
		return baseTools
	})

	agentManager.SetCommandFactory(func(a *agent.Agent) []interfaces.Command {
		var cmds []interfaces.Command
		// Get skill metas to create commands for user-invocable skills
		for _, meta := range skillManager.ListMeta(context.Background()) {
			if meta.UserInvocable {
				cmds = append(cmds, skill.NewSkillCommand(skillManager, meta))
			}
		}
		return cmds
	})

	// 6. 初始化 Gateway
	logger.L().Debug().Msg("Initializing Gateway...")
	gwCfg := config.Get().Gateway
	gw := gateway.NewGateway(
		gateway.WithRouter(gateway.NewRouter(gwCfg.Routes, agentManager)),
		gateway.WithAgentManager(agentManager),
		gateway.WithChannelManager(channelManager),
		gateway.WithMessageBus(msgBus),
	)

	// 7. 启动 Gateway 和 Channel
	logger.L().Debug().Msg("Starting Gateway and Channel...")
	if err := gw.Start(ctx); err != nil {
		return err
	}

	if err := channelManager.StartAll(ctx); err != nil {
		return err
	}

	// 9. 启动 Web HTTP Server
	var httpSrv *httpserver.Server
	if cfg.Web.Enabled {
		webChan := httpserver.RegisterWebChannel(msgBus)
		if err := channelManager.Add(ctx, "web", webChan); err != nil {
			logger.L().Error().Err(err).Msg("Failed to register WebChannel")
		} else {
			if err := webChan.Start(ctx); err != nil {
				logger.L().Error().Err(err).Msg("Failed to start WebChannel")
			}
		}
		httpSrv = httpserver.NewServer(msgBus, agentManager)
		if err := httpSrv.Start(ctx); err != nil {
			logger.L().Error().Err(err).Msg("Failed to start HTTP server")
		}
	}

	// 8. 启动 CronService
	if cfg.Tools.Cron.Enabled && cronService != nil {
		logger.L().Debug().Msg("Starting CronService...")
		if err := cronService.Start(ctx); err != nil {
			logger.L().Error().Err(err).Msg("Failed to start CronService")
		}
	}

	// Get the default agent
	defaultAgent, err := agentManager.Get(context.Background(), interfaces.DefaultAgentName)
	if err != nil {
		return err
	}

	// 等待 Gateway 准备好
	time.Sleep(100 * time.Millisecond)

	// 设置信号处理
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// 设置初始 session
	currentChatID := util.GenerateRandomChatID()

	// 交互式与非交互式执行
	// 非交互式仅作调试使用，绕开 tui 前端
	if agentMessage != "" {
		// 非交互式执行
		inboundMsg := &bus.InBoundMessage{
			InChannel: "cli",
			SenderID:  "cli",
			ChatID:    currentChatID,
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
		// ========== Interactive execution ==========

		// 创建 EventBus 并注册 observers
		tvCfg := cfg.Hooks.ThinkingVisibility
		var eventBus *events.EventBus
		if tvCfg.Enabled {
			eventBus = events.NewEventBus(tvCfg.MaxEvents)
			hook.RegisterObservers(eventBus, tvCfg)
		}

		model := tui.NewAppModel(msgBus, defaultAgent)
		p := tea.NewProgram(model, tea.WithAltScreen())

		// 获取 CLIChannel 并设置回调
		var responseChan chan responsePayload = make(chan responsePayload, 1)
		cliChan, err := channelManager.Get(ctx, "cli")
		if err == nil {
			if c, ok := cliChan.(*channel.CLIChannel); ok {
				c.SetOnResponse(func(content, msgType string) {
					responseChan <- responsePayload{Content: content, MsgType: msgType}
				})
			}
		}

		// 响应通道 -> TUI
		go func() {
			for payload := range responseChan {
				p.Send(tui.AgentResponseMsg{Content: payload.Content, MsgType: payload.MsgType})
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
			fmt.Fprintf(os.Stderr, "TUI error: %v\n", err)
		}

		close(responseChan)

		// Stop
		fmt.Println("Shutting down... Hope to see you next time!😊")
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer stopCancel()
		if httpSrv != nil {
			if err := httpSrv.Stop(stopCtx); err != nil {
				logger.L().Error().Err(err).Msg("Failed to stop HTTP server")
			}
		}
		if err := gw.Stop(stopCtx); err != nil {
			logger.L().Error().Err(err).Msg("Failed to stop Gateway")
		}
		if cfg.Tools.Cron.Enabled && cronService != nil {
			if err := cronService.Stop(stopCtx); err != nil {
				logger.L().Error().Err(err).Msg("Failed to stop CronService")
			}
		}
		if err := channelManager.StopAll(ctx); err != nil {
			logger.L().Error().Err(err).Msg("Failed to stop ChannelManager")
		}
		if err := agentManager.StopAll(ctx); err != nil {
			logger.L().Error().Err(err).Msg("Failed to stop AgentManager")
		}
		os.Exit(0)
		return nil
	}
	return nil
}
