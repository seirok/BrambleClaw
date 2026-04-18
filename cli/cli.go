package cli

import (
	"brambleclaw/agent"
	"brambleclaw/bus"
	"brambleclaw/channel"
	"brambleclaw/config"
	"brambleclaw/gateway"
	"brambleclaw/logger"
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
	agentCmd.Flags().StringVarP(&agentSession, "session", "s", "cli:default", "指定 Session Key，保留上下文对话")
	agentCmd.AddCommand(agentNewCmd)
	rootCmd.AddCommand(agentCmd)

	debugCmd.Flags().IntVarP(&debugLines, "lines", "n", 100, "输出最近的日志行数")
	debugCmd.Flags().BoolVarP(&debugSession, "session", "s", false, "分析 session")
	rootCmd.AddCommand(debugCmd)
}

func Execute() error {
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

	cfg := config.Config{}
	cfg.Log = config.LogConfig{
		Path:           "logs/brambleclaw.log",
		ConsoleEnabled: false,
		Level:          "debug",
	}
	cfg.BusBufSize = 500
	cfg.SubBufSize = 100
	cfg.Channels.CLI.Enabled = true
	cfg.Channels.CLI.AllowedIDs = []string{"user"}
	cfg.LLMConfig = config.LLMConfig{
		APIKey:  apiKey,
		BaseURL: baseUrl,
		Model:   model,
	}
	cfg.Tools.WebSearch.Enabled = false
	cfg.Tools.UrlParse.Enabled = false
	cfg.Tools.MCP.Enabled = false
	// 设置默认的 Gateway 配置
	cfg.Gateway = config.DefaultGatewayConfig()

	// 确定配置文件保存路径（优先保存到用户配置目录）
	var configPath string
	if userConfigDir, err := os.UserConfigDir(); err == nil {
		configDir := filepath.Join(userConfigDir, config.AppName)
		if err := os.MkdirAll(configDir, 0755); err == nil {
			configPath = filepath.Join(configDir, config.DefaultConfigFileName)
		}
	}
	// 如果用户配置目录不可用，退回到当前工作目录
	if configPath == "" {
		if err := os.MkdirAll(config.DefaultConfigDir, 0755); err != nil {
			err = fmt.Errorf("创建配置目录失败(%s): %w", config.DefaultConfigDir, err)
			logger.L().Error().Err(err).Msg("")
			return err
		}
		configPath = filepath.Join(config.DefaultConfigDir, config.DefaultConfigFileName)
	}

	file, err := os.Create(configPath)
	if err != nil {
		err = fmt.Errorf("创建配置文件失败(%s): %w", configPath, err)
		logger.L().Error().Err(err).Msg("")
		return err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(cfg); err != nil {
		err = fmt.Errorf("保存配置失败(%s): %w", configPath, err)
		logger.L().Error().Err(err).Msg("")
		return err
	}

	fmt.Printf("配置已成功保存至 %s\n", configPath)
	logger.L().Debug().Str("config_path", configPath).Str("action", "init").Msg("初始化配置完成")
	return nil
}

func runDebug(cmd *cobra.Command, args []string) error {
	// 如果指定了 -s 或 --session，运行 session 分析器
	if debugSession {
		return runDebugSessions()
	}

	logger.L().Debug().Msg("加载配置并初始化日志分析器...")

	loader := config.NewLoader()
	cfg, _, err := loader.Load()
	if err != nil {
		return err
	}

	logPath := cfg.Log.Path
	if logPath == "" {
		logPath = "logs/brambleclaw.log"
	}

	logger.L().Debug().Str("log_path", logPath).Int("lines", debugLines).Msg("开始格式化输出日志")

	err = logger.AnalyzeLogs(logPath, debugLines)
	if err != nil {
		return err
	}
	return nil
}

func runAgent(cmd *cobra.Command, args []string) error {
	logger.L().Debug().Msg("加载系统配置...")

	// 使用配置加载器，支持多路径搜索
	loader := config.NewLoader()
	cfg, configPath, err := loader.Load()
	if err != nil {
		return err // 底层已经包装，直接透传
	}

	// 根据配置重新初始化日志
	logger.Setup(cfg.Log.Path, cfg.Log.Level, cfg.Log.ConsoleEnabled)
	logger.L().Debug().Str("config_path", configPath).Msg("成功加载配置文件")

	// 配置项检查

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 1. 初始化消息总线
	logger.L().Debug().Msg("初始化消息总线...")
	msgBus := bus.NewMessageBus(cfg.BusBufSize)

	// 2. 初始化通道管理器
	logger.L().Debug().Msg("初始化通道管理器...")
	channelManager := channel.NewManager(msgBus)

	// 3. 初始化 CLI 通道
	logger.L().Debug().Msg("初始化 CLI 通道...")
	cliCfg := &channel.BaseChannelConfig{
		Enabled:    cfg.Channels.CLI.Enabled,
		AllowedIDs: cfg.Channels.CLI.AllowedIDs,
	}
	cliChan := channel.NewCLIChannel(cliCfg, msgBus)
	if err = channelManager.Register(cliChan); err != nil {
		return err
	}

	// 4. 加载 Gateway 配置
	logger.L().Debug().Msg("加载 Gateway 配置...")
	gwCfg, err := gateway.LoadConfigFromConfig(cfg)
	if err != nil {
		return err // 底层已经包装，直接透传
	}

	// 5. 初始化 Gateway
	logger.L().Debug().Msg("初始化 Gateway...")
	gw := gateway.NewGateway(gwCfg, msgBus, channelManager)

	// 6. 恢复 Agent 并重新注册
	logger.L().Debug().Msg("[runAgent] 恢复 Agent 并重新注册...")
	if err = gw.RegisterAgents(cfg); err != nil {
		return err
	}

	// 启动 agents
	agentRegistry := gw.GetRegistry()
	agents := agentRegistry.List()
	for _, agentName := range agents {
		aGent, _ := agentRegistry.GetAgent(agentName)
		err = aGent.Start(ctx)
		if err != nil {
			return err
		}
	}

	// 7. 启动 Gateway 和 Channel
	logger.L().Debug().Msg("启动 Gateway 和 Channel...")
	if err = gw.Start(ctx); err != nil {
		return err
	}

	if err = channelManager.Start(ctx); err != nil {
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
				logger.L().Error().Err(err).Msg("发送消息失败")
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
	gw.Stop()
	if err = channelManager.Stop(); err != nil {
		return err
	}
	agentNames := agentRegistry.List()
	for _, agentName := range agentNames {
		aGent, _ := agentRegistry.GetAgent(agentName)
		aGent.Stop()
	}
	return nil
}

// runAgentNew 运行 Agent 创建向导
func runAgentNew(cmd *cobra.Command, args []string) error {
	logger.L().Debug().Msg("加载配置...")

	// 加载配置
	loader := config.NewLoader()
	cfg, cfgPath, err := loader.Load()
	if err != nil {
		return err
	}

	// 创建 Agent 创建向导
	creator := NewAgentCreator(cfg, cfgPath)
	return creator.Run()
}

// runDebugSessions 运行 session 分析器
func runDebugSessions() error {
	fmt.Println("============================================")
	fmt.Println("        Session 分析器")
	fmt.Println("============================================")
	fmt.Println()

	// 使用配置加载器，支持多路径搜索
	loader := config.NewLoader()
	cfg, _, err := loader.Load()
	if err != nil {
		return err // 底层已经包装，直接透传
	}

	// 根据配置重新初始化日志
	logger.Setup(cfg.Log.Path, cfg.Log.Level, cfg.Log.ConsoleEnabled)

	// 检查配置文件
	if err = cfg.CheckConfig(); err != nil {
		return err
	}

	// 创建分析器
	analyzer := agent.NewSessionAnalyzer(cfg.Workspace)

	// 分析所有 session
	infos, err := analyzer.AnalyzeAll()
	if err != nil {
		return fmt.Errorf("分析 session 失败: %w", err)
	}

	// 打印结果
	agent.PrintSessionInfo(infos)

	return nil
}
