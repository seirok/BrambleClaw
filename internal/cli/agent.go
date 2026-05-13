package cli

import (
	util "brambleclaw/internal"
	"brambleclaw/internal/agent"
	"brambleclaw/internal/bus"
	"brambleclaw/internal/channel"
	"brambleclaw/internal/cli/tui"
	"brambleclaw/internal/config"
	"brambleclaw/internal/events"
	"brambleclaw/internal/gateway"
	"brambleclaw/internal/hook"
	"brambleclaw/internal/interfaces"
	"brambleclaw/internal/logger"
	"brambleclaw/internal/runtime"
	"brambleclaw/internal/session"
	"brambleclaw/internal/skill"
	"brambleclaw/internal/teamtool"
	"brambleclaw/internal/tools"
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
)

// toolAdapter adapts skill.Tool to tools.Tool
type toolAdapter struct {
	name        string
	description string
	execute     func(ctx context.Context, args string) (interface{}, error)
	parameters  map[string]interface{}
}

func (a *toolAdapter) Name() string {
	return a.name
}

func (a *toolAdapter) Description() string {
	return a.description
}

func (a *toolAdapter) Execute(ctx context.Context, args string) (interface{}, error) {
	return a.execute(ctx, args)
}

func (a *toolAdapter) Parameters() map[string]interface{} {
	return a.parameters
}

var agentCmd = &cobra.Command{
	Use:   "agent",
	Short: "Start AI agent service",
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
		return fmt.Errorf("failed to load system configuration")
	}

	logger.Setup(cfg.Log.Level, cfg.Log.ConsoleEnabled)
	logger.L().Debug().Msg("Log configuration loaded")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 1. Initialize message bus
	logger.L().Debug().Msg("Initializing message bus...")
	msgBus := bus.NewMessageBus(cfg.BusBufSize)

	// 2. Initialize channel manager
	logger.L().Debug().Msg("Initializing ChannelManager...")
	channelManager := channel.NewChannelManager(msgBus)
	logger.L().Debug().Msg("Initializing channel manager with config...")
	if err := channelManager.Initialize(ctx, cfg); err != nil {
		return fmt.Errorf("failed to initialize channel manager: %w", err)
	}

	// 5. Initialize SkillManager and AgentManager
	logger.L().Debug().Msg("Initializing SkillManager...")
	skillManager := skill.NewSkillManager(&cfg.Skill)

	logger.L().Debug().Msg("Initializing AgentManager...")
	agentManager := agent.NewAgentManager(msgBus, runtime.NewAgentRuntime())
	agentManager.SetSkillManager(skillManager)

	agentManager.SetToolFactory(func(a *agent.Agent) []tools.Tool {
		baseTools := []tools.Tool{teamtool.NewCreateTeamTool(a)}
		// Add activate_skill tool with adapter
		skTool := skill.NewActivateSkillTool(skillManager)
		adapted := &toolAdapter{
			name:        skTool.Name(),
			description: skTool.Description(),
			execute:     skTool.Execute,
			parameters:  skTool.Parameters(),
		}
		baseTools = append(baseTools, adapted)
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

	// 6. Initialize Gateway
	logger.L().Debug().Msg("Initializing Gateway...")
	gwCfg := config.Get().Gateway
	gw := gateway.NewGateway(
		gateway.WithRouter(gateway.NewRouter(gwCfg.Routes, agentManager)),
		gateway.WithAgentManager(agentManager),
		gateway.WithChannelManager(channelManager),
		gateway.WithMessageBus(msgBus),
	)

	// 7. Start Gateway and Channel
	logger.L().Debug().Msg("Starting Gateway and Channel...")
	if err := gw.Start(ctx); err != nil {
		return err
	}

	if err := channelManager.StartAll(ctx); err != nil {
		return err
	}

	// Get the default agent to access its session manager
	var sessMgr *session.PersistentSessionManager
	agentName := "main"

	// List agents and get the first one's session manager
	agents := agentManager.List(context.Background())
	if len(agents) > 0 {
		a := agents[0]
		sessMgr = a.SessionMgr()
		agentName = a.Name()
	}

	// Fallback: if no agent or can't get session manager, create one
	if sessMgr == nil {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			homeDir = "."
		}
		workDir := filepath.Join(homeDir, ".brambleclaw", agentName)
		sessMgr = session.NewPersistentSessionManager(workDir)
	}

	// Wait for Gateway to be ready
	time.Sleep(100 * time.Millisecond)

	// Set up signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// ========== Interactive execution ==========

	// 创建 EventBus 并注册 observers
	tvCfg := cfg.Hooks.ThinkingVisibility
	var eventBus *events.EventBus
	if tvCfg.Enabled {
		eventBus = events.NewEventBus(tvCfg.MaxEvents)
		hook.RegisterObservers(eventBus, tvCfg)
	}

	// 设置初始 session
	currentChatID := util.GenerateRandomChatID()

	model := tui.NewAppModel(msgBus, currentChatID, sessMgr, agentName)
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
		fmt.Fprintf(os.Stderr, "TUI error: %v\n", err)
	}

	close(responseChan)

	// Stop
	fmt.Println("Shutting down... Hope to see you next time!😊")
	stopCtx, stopCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer stopCancel()
	if err := gw.Stop(stopCtx); err != nil {
		logger.L().Error().Err(err).Msg("Failed to stop Gateway")
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
