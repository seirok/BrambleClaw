package cli

import (
	"brambleclaw/internal/agent"
	"brambleclaw/internal/bus"
	channel2 "brambleclaw/internal/channel"
	config2 "brambleclaw/internal/config"
	"brambleclaw/internal/gateway"
	logger2 "brambleclaw/internal/logger"
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
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

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "初始化配置，提供配置向导",
	RunE:  runInit,
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

var agentNewCmd = &cobra.Command{
	Use:   "new",
	Short: "创建新 Agent",
	RunE:  runAgentNew,
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
	rootCmd.AddCommand(initCmd)

	agentCmd.Flags().StringVarP(&agentMessage, "message", "m", "", "非交互式执行：发送一条消息后退出")
	agentCmd.Flags().StringVarP(&agentSession, "session", "s", "default", "指定 Session Key，保留上下文对话")
	agentCmd.AddCommand(agentNewCmd)
	rootCmd.AddCommand(agentCmd)

	debugCmd.Flags().IntVarP(&debugLines, "lines", "n", 100, "输出最近的日志行数")
	debugCmd.Flags().BoolVarP(&debugSession, "session", "s", false, "分析 session")
	rootCmd.AddCommand(debugCmd)
}

func Execute() error {
	// 初始化配置单例（在程序启动时只执行一次）
	if err := config2.Init(""); err != nil {
		logger2.L().Error().Err(err).Msg("配置初始化失败")
		return err
	}
	logger2.L().Debug().Str("path", config2.GetPath()).Msg("配置单例初始化成功")

	err := rootCmd.Execute()
	if err != nil {
		logger2.L().Error().Err(err).Msg("命令执行失败")
	}
	return err
}

func runVersion(cmd *cobra.Command, args []string) {
	fmt.Println("brambleclaw 1.0.0")
	fmt.Println("License: MIT")
}

func runInit(cmd *cobra.Command, args []string) error {
	fmt.Println("=== brambleclaw 初始化向导 ===")
	scanner := bufio.NewScanner(os.Stdin)

	fmt.Print("请输入 LLM API Key: ")
	scanner.Scan()
	apiKey := scanner.Text()

	fmt.Print("请输入 Base URL (例如: https://api.deepseek.com/v1/chat/completions): ")
	scanner.Scan()
	baseUrl := scanner.Text()
	if baseUrl == "" {
		baseUrl = "https://api.deepseek.com/v1/chat/completions"
	}

	fmt.Print("请输入模型名称 (例如: deepseek-chat): ")
	scanner.Scan()
	model := scanner.Text()
	if model == "" {
		model = "deepseek-chat"
	}

	cfg := config2.Config{}
	cfg.Log = config2.LogConfig{
		Path:           "logs/brambleclaw.log",
		ConsoleEnabled: false,
		Level:          "debug",
	}
	cfg.BusBufSize = 500
	cfg.SubBufSize = 100
	cfg.Channels.CLI.Enabled = true
	cfg.Channels.CLI.AllowedIDs = []string{"user"}
	cfg.LLMConfig = config2.LLMConfig{
		APIKey:  apiKey,
		BaseURL: baseUrl,
		Model:   model,
	}
	cfg.Tools.WebSearch.Enabled = false
	cfg.Tools.UrlParse.Enabled = false
	cfg.Tools.MCP.Enabled = false
	// 设置默认的 Gateway 配置
	cfg.Gateway = config2.DefaultGatewayConfig()

	// 确定配置文件保存路径（优先保存到用户配置目录）
	var configPath string
	if userConfigDir, err := os.UserConfigDir(); err == nil {
		configDir := filepath.Join(userConfigDir, config2.AppName)
		if err := os.MkdirAll(configDir, 0755); err == nil {
			configPath = filepath.Join(configDir, config2.DefaultConfigFileName)
		}
	}
	// 如果用户配置目录不可用，退回到当前工作目录
	if configPath == "" {
		if err := os.MkdirAll(config2.DefaultConfigDir, 0755); err != nil {
			err = fmt.Errorf("创建配置目录失败(%s): %w", config2.DefaultConfigDir, err)
			logger2.L().Error().Err(err).Msg("")
			return err
		}
		configPath = filepath.Join(config2.DefaultConfigDir, config2.DefaultConfigFileName)
	}

	file, err := os.Create(configPath)
	if err != nil {
		err = fmt.Errorf("创建配置文件失败(%s): %w", configPath, err)
		logger2.L().Error().Err(err).Msg("")
		return err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(cfg); err != nil {
		err = fmt.Errorf("保存配置失败(%s): %w", configPath, err)
		logger2.L().Error().Err(err).Msg("")
		return err
	}

	fmt.Printf("配置已成功保存至 %s\n", configPath)
	logger2.L().Debug().Str("config_path", configPath).Str("action", "init").Msg("初始化配置完成")
	return nil
}

func runDebug(cmd *cobra.Command, args []string) error {
	// 如果指定了 -s 或 --session，运行 session 分析器
	if debugSession {
		return runDebugSessions()
	}

	logger2.L().Debug().Msg("加载配置并初始化日志分析器...")

	cfg := config2.Get()
	if cfg == nil {
		return fmt.Errorf("配置未初始化")
	}

	logPath := cfg.Log.Path
	if logPath == "" {
		logPath = "logs/brambleclaw.log"
	}

	logger2.L().Debug().Str("log_path", logPath).Int("lines", debugLines).Msg("开始格式化输出日志")

	err := logger2.AnalyzeLogs(logPath, debugLines)
	if err != nil {
		return err
	}
	return nil
}

func runAgent(cmd *cobra.Command, args []string) error {
	logger2.L().Debug().Msg("加载系统配置...")

	cfg := config2.Get()
	if cfg == nil {
		return fmt.Errorf("配置未初始化")
	}

	logger2.Setup(cfg.Log.Path, cfg.Log.Level, cfg.Log.ConsoleEnabled)
	logger2.L().Debug().Str("config_path", config2.GetPath()).Msg("成功加载配置文件")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 1. 初始化消息总线
	logger2.L().Debug().Msg("初始化消息总线...")
	msgBus := bus.NewMessageBus(cfg.BusBufSize)

	// 2. 初始化通道管理器
	logger2.L().Debug().Msg("初始化通道管理器...")
	channelManager := channel2.NewChannelManager(msgBus)
	logger2.L().Debug().Msg("Initializing channel manager with config...")
	if err := channelManager.Initialize(ctx, cfg); err != nil {
		return fmt.Errorf("failed to initialize channel manager: %w", err)
	}

	// 3. 加载 Gateway 配置
	logger2.L().Debug().Msg("加载 Gateway 配置...")
	gwCfg, err := gateway.LoadConfigFromConfig(cfg)
	if err != nil {
		return err // 底层已经包装，直接透传
	}

	// 5. 初始化 AgentManager
	logger2.L().Debug().Msg("初始化 AgentManager...")
	agentManager := agent.NewAgentManager(msgBus)

	// 6. 初始化 Gateway
	logger2.L().Debug().Msg("初始化 Gateway...")
	gw := gateway.NewGateway(
		gateway.WithRouter(gateway.NewRouter(gwCfg.Routes, agentManager)),
		gateway.WithAgentManager(agentManager),
		gateway.WithChannelManager(channelManager),
		gateway.WithMessageBus(msgBus),
	)

	// 7. 启动 Gateway 和 Channel
	logger2.L().Debug().Msg("启动 Gateway 和 Channel...")
	if err = gw.Start(ctx); err != nil {
		return err
	}

	if err = channelManager.StartAll(ctx); err != nil {
		return err
	}

	// 等待 Gateway 准备好
	time.Sleep(100 * time.Millisecond)

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
				return nil
			}
		case <-time.After(30 * time.Second):
			return fmt.Errorf("执行超时")
		case <-ctx.Done():
			return nil
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

		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			input := scanner.Text()
			if input == "exit" || input == "quit" {
				break
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
				logger2.L().Error().Err(err).Msg("发送消息失败")
				continue
			}
		}
	}

	// 等待信号退出
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	if agentMessage == "" {
		<-sigChan
	}

	// 停止
	fmt.Println("Shutting down... Hope to see you next time!😊")
	if err = gw.Stop(ctx); err != nil {
		logger2.L().Error().Err(err).Msg("停止 Gateway 失败")
	}
	if err = channelManager.StopAll(ctx); err != nil {
		logger2.L().Error().Err(err).Msg("停止 ChannelManager 失败")
	}
	if err = agentManager.StopAll(ctx); err != nil {
		logger2.L().Error().Err(err).Msg("停止 AgentManager 失败")
	}
	return nil
}

// runAgentNew 运行 Agent 创建向导
func runAgentNew(cmd *cobra.Command, args []string) error {
	logger2.L().Debug().Msg("加载配置...")

	// 从单例获取配置
	cfg := config2.Get()
	if cfg == nil {
		return fmt.Errorf("配置未初始化")
	}

	// 创建 Agent 创建向导
	creator := NewAgentCreator(cfg, config2.GetPath())
	return creator.Run()
}

// runDebugSessions 运行 session 分析器
func runDebugSessions() error {
	fmt.Println("============================================")
	fmt.Println("        Session 分析器")
	fmt.Println("============================================")
	fmt.Println()

	// 从单例获取配置
	cfg := config2.Get()
	if cfg == nil {
		return fmt.Errorf("配置未初始化")
	}

	// 根据配置重新初始化日志
	logger2.Setup(cfg.Log.Path, cfg.Log.Level, cfg.Log.ConsoleEnabled)

	// 检查配置文件
	if err := cfg.CheckConfig(); err != nil {
		return err
	}

	// TODO:创建分析器

	return nil
}
