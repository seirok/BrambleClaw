package cli

import (
	"context"
	"fmt"
	"neoclaw/internal/agent"
	"neoclaw/internal/bus"
	"neoclaw/internal/channel"
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

	"github.com/spf13/cobra"
)

type initializedServices struct {
	ctx            context.Context
	cancel         context.CancelFunc
	msgBus         *bus.MessageBus
	agentManager   *agent.AgentManager
	gateway        *gateway.Gateway
	httpSrv        *httpserver.Server
	cronService    *cron.CronService
	channelManager *channel.ChannelManager
	webChan        *httpserver.WebChannel
	eventBus       *events.EventBus
	defaultAgent   *agent.Agent
}

var webCmd = &cobra.Command{
	Use:   "web",
	Short: "启动 Web 前端服务（仅 HTTP Server，无 TUI）",
	RunE:  runWeb,
}

func init() {
	rootCmd.AddCommand(webCmd)
}

func runWeb(cmd *cobra.Command, args []string) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	svcs, err := initializeServices(ctx)
	if err != nil {
		return err
	}

	logger.L().Info().Msg("Web server running. Press Ctrl+C to stop.")

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	logger.L().Info().Msg("Shutting down...")

	stopCtx, stopCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer stopCancel()

	if svcs.httpSrv != nil {
		if err := svcs.httpSrv.Stop(stopCtx); err != nil {
			logger.L().Error().Err(err).Msg("Failed to stop HTTP server")
		}
	}
	if err := svcs.gateway.Stop(stopCtx); err != nil {
		logger.L().Error().Err(err).Msg("Failed to stop Gateway")
	}
	if svcs.cronService != nil {
		if err := svcs.cronService.Stop(stopCtx); err != nil {
			logger.L().Error().Err(err).Msg("Failed to stop CronService")
		}
	}
	if err := svcs.channelManager.StopAll(ctx); err != nil {
		logger.L().Error().Err(err).Msg("Failed to stop ChannelManager")
	}
	if err := svcs.agentManager.StopAll(ctx); err != nil {
		logger.L().Error().Err(err).Msg("Failed to stop AgentManager")
	}

	fmt.Println("Shutdown complete.")
	return nil
}

func initializeServices(ctx context.Context) (*initializedServices, error) {
	cfg := config.Get()
	if cfg == nil {
		return nil, fmt.Errorf("系统配置加载失败")
	}

	logger.Setup(cfg.Log.Level, cfg.Log.ConsoleEnabled)

	ctx, cancel := context.WithCancel(ctx)

	// 1. MessageBus
	msgBus := bus.NewMessageBus(cfg.BusBufSize)

	// 2. ChannelManager
	channelManager := channel.NewChannelManager(msgBus)
	if err := channelManager.Initialize(ctx, cfg); err != nil {
		cancel()
		return nil, fmt.Errorf("failed to initialize channel manager: %w", err)
	}

	// 3. Cron (optional)
	var cronService *cron.CronService
	var cronTool *cron.CronTool
	if cfg.Tools.Cron.Enabled {
		cronService, cronTool = cron.NewCronServiceAndTool(msgBus, cfg.Tools.Cron.DataDir)
	}

	// 4. SkillManager + AgentManager
	skillManager := skill.NewSkillManager(&cfg.Skill)
	agentManager := agent.NewAgentManager(msgBus, runtime.NewAgentRuntime())
	agentManager.SetSkillManager(skillManager)

	agentManager.SetToolFactory(func(a *agent.Agent) []interfaces.Tool {
		baseTools := []interfaces.Tool{teamtool.NewCreateTeamTool(a)}
		if cfg.Tools.Cron.Enabled && cronTool != nil {
			baseTools = append(baseTools, cronTool)
		}
		skTool := skill.NewActivateSkillTool(skillManager)
		baseTools = append(baseTools, skTool)
		return baseTools
	})

	agentManager.SetCommandFactory(func(a *agent.Agent) []interfaces.Command {
		var cmds []interfaces.Command
		for _, meta := range skillManager.ListMeta(context.Background()) {
			if meta.UserInvocable {
				cmds = append(cmds, skill.NewSkillCommand(skillManager, meta))
			}
		}
		return cmds
	})

	// 5. Gateway
	gwCfg := config.Get().Gateway
	gw := gateway.NewGateway(
		gateway.WithRouter(gateway.NewRouter(gwCfg.Routes, agentManager)),
		gateway.WithAgentManager(agentManager),
		gateway.WithChannelManager(channelManager),
		gateway.WithMessageBus(msgBus),
	)

	if err := gw.Start(ctx); err != nil {
		cancel()
		return nil, err
	}

	if err := channelManager.StartAll(ctx); err != nil {
		cancel()
		return nil, err
	}

	// 6. Web HTTP Server
	var httpSrv *httpserver.Server
	var webChan *httpserver.WebChannel
	if cfg.Web.Enabled {
		webChan = httpserver.RegisterWebChannel(msgBus)
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

	// 7. CronService
	if cfg.Tools.Cron.Enabled && cronService != nil {
		logger.L().Debug().Msg("Starting CronService...")
		if err := cronService.Start(ctx); err != nil {
			logger.L().Error().Err(err).Msg("Failed to start CronService")
		}
	}

	// 8. Default agent
	defaultAgent, err := agentManager.Get(context.Background(), interfaces.DefaultAgentName)
	if err != nil {
		cancel()
		return nil, err
	}

	// 9. EventBus (for thinking visibility hooks)
	var eventBus *events.EventBus
	tvCfg := cfg.Hooks.ThinkingVisibility
	if tvCfg.Enabled {
		eventBus = events.NewEventBus(tvCfg.MaxEvents)
		hook.RegisterObservers(eventBus, tvCfg)
	}

	time.Sleep(100 * time.Millisecond)

	return &initializedServices{
		ctx:            ctx,
		cancel:         cancel,
		msgBus:         msgBus,
		agentManager:   agentManager,
		gateway:        gw,
		httpSrv:        httpSrv,
		cronService:    cronService,
		channelManager: channelManager,
		webChan:        webChan,
		eventBus:       eventBus,
		defaultAgent:   defaultAgent,
	}, nil
}
