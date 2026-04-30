package cli

import (
	util "brambleclaw/internal"
	"brambleclaw/internal/agent"
	"brambleclaw/internal/bus"
	"brambleclaw/internal/channel"
	"brambleclaw/internal/config"
	"brambleclaw/internal/gateway"
	"brambleclaw/internal/logger"
	"bufio"
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "brambleclaw",
	Short: "Go-based AI Agent framework",
	Long:  `brambleclaw is a Go language implementation of an AI Agent framework.`,
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Run:   runVersion,
}

var agentCmd = &cobra.Command{
	Use:   "agent",
	Short: "启动 AI 代理服务",
	RunE:  runAgent,
}

var debugCmd = &cobra.Command{
	Use:   "debug",
	Short: "输出并格式化最近的日志和分析 session",
	RunE:  runDebug,
}

// 选项
var (
	agentMessage string
	agentSession string
	debugLines   int
	debugSession bool
)

func init() {
	rootCmd.AddCommand(versionCmd)

	agentCmd.Flags().StringVarP(&agentMessage, "message", "m", "", "非交互式执行：发送一条消息后退出")
	agentCmd.Flags().StringVarP(&agentSession, "session", "s", "default", "指定 Session Key，保留上下文对话")

	rootCmd.AddCommand(agentCmd)

	debugCmd.Flags().IntVarP(&debugLines, "lines", "n", 100, "输出最近的日志行数")
	debugCmd.Flags().BoolVarP(&debugSession, "session", "s", false, "分析 session")
	rootCmd.AddCommand(debugCmd)
}

func Execute() error {
	// 初始化配置单例
	config.Init()
	logger.L().Debug().Msg("配置加载成功")

	err := rootCmd.Execute()
	if err != nil {
		logger.L().Error().Err(err).Msg("命令执行失败")
	}
	return err
}

func runVersion(cmd *cobra.Command, args []string) {
	fmt.Println("brambleclaw 1.0.0")
	fmt.Println("License: MIT")
}

func runDebug(cmd *cobra.Command, args []string) error {
	// 如果指定了 -s 或 --session，运行 session 分析器
	if debugSession {
		return runDebugSessions()
	}

	logPath := util.GetLogPath()
	logger.L().Debug().Str("log_path", logPath).Int("lines", debugLines).Msg("开始格式化输出日志")

	err := logger.AnalyzeLogs(logPath, debugLines)
	if err != nil {
		return err
	}
	return nil
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
	agentManager := agent.NewAgentManager(msgBus)

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

	// 交互式与非交互式模式
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
				// Gateway 输出已通过 CLIChannel 的 Send 方法打印了，
				// 这里只需要等待执行完成即可退出
				time.Sleep(100 * time.Millisecond)
			}
		case <-time.After(30 * time.Second):
			return fmt.Errorf("执行超时")
		case <-ctx.Done():
		case <-sigChan:
			fmt.Println("\n收到退出信号...")
		}
	} else {
		// 交互式执行
		color1 := "\033[38;2;138;43;226m"
		color2 := "\033[38;2;112;60;212m"
		color3 := "\033[38;2;87;77;199m"
		color4 := "\033[38;2;62;93;185m"
		resetColor := "\033[0m"
		banner :=
			color1 + `██████╗ ██████╗  █████╗ ███╗   ███╗██████╗ ██╗     ███████╗    ██████╗██╗       █████╗ ██╗    ██╗` + "\n" +
				color2 + `██╔══██╗██╔══██╗██╔══██╗████╗ ████║██╔══██╗██║     ██╔════╝   ██╔════╝██║      ██╔══██╗██║    ██║` + "\n" +
				color2 + `██████╔╝██████╔╝███████║██╔████╔██║██████╔╝██║     █████╗     ██║     ██║      ███████║██║ █╗ ██║` + "\n" +
				color3 + `██╔══██╗██╔══██╗██╔══██║██║╚██╔╝██║██╔══██╗██║     ██╔══╝     ██║     ██║      ██╔══██║██║███╗██║` + "\n" +
				color3 + `██████╔╝██║  ██║██║  ██║██║ ╚═╝ ██║██████╔╝███████╗███████╗    ╚██████╗███████╗██║  ██║╚███╔███╔╝` + "\n" +
				color4 + `╚═════╝ ╚═╝  ╚═╝╚═╝  ╚═╝╚═╝     ╚═╝╚═════╝ ╚══════╝╚══════╝     ╚═════╝╚══════╝╚═╝  ╚═╝ ╚══╝╚══╝ ` + resetColor
		fmt.Println(banner)
		fmt.Println("\n" + color4 + "    >>> Welcome to brambleclaw System. <<<" + resetColor)
		fmt.Println("> 你好，请问有什么可以帮您？")

		// 使用 goroutine 处理输入
		inputChan := make(chan string, 1)
		go func() {
			scanner := bufio.NewScanner(os.Stdin)
			for scanner.Scan() {
				inputChan <- scanner.Text()
			}
			close(inputChan)
		}()

		// 主循环：处理输入和信号
	mainLoop:
		for {
			select {
			case input, ok := <-inputChan:
				if !ok {
					break mainLoop
				}
				if input == "exit" || input == "quit" {
					break mainLoop
				}
				if input == "" {
					continue
				}

				inboundMsg := &bus.InBoundMessage{
					InChannel: "cli",
					SenderID:  "cli",
					ChatID:    agentSession,
					Content:   input,
					TimeStamp: time.Now(),
				}

				if err := msgBus.PublishInBoundMessage(ctx, inboundMsg); err != nil {
					logger.L().Error().Err(err).Msg("发送消息失败")
					continue
				}
			case <-sigChan:
				fmt.Println("\n收到退出信号...")
				break mainLoop
			}
		}
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

// runDebugSessions 运行 session 分析器
func runDebugSessions() error {
	fmt.Println("============================================")
	fmt.Println("        Session 分析器")
	fmt.Println("============================================")
	fmt.Println()

	// 从单例获取配置
	cfg := config.Get()
	if cfg == nil {
		return fmt.Errorf("配置未初始化")
	}

	// 根据配置重新初始化日志
	logger.Setup(cfg.Log.Level, cfg.Log.ConsoleEnabled)

	// TODO:创建分析器

	return nil
}
