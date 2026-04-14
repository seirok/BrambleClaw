package cli

import (
	"brambleclaw/agent"
	"brambleclaw/bus"
	"brambleclaw/channel"
	"brambleclaw/config"
	"brambleclaw/gateway"
	"brambleclaw/logger"
	"brambleclaw/tools"
	"bufio"
	"context"
	"encoding/json"
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

// 选项
var (
	agentMessage string
	agentSession string
)

func init() {
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(initCmd)

	agentCmd.Flags().StringVarP(&agentMessage, "message", "m", "", "非交互式执行：发送一条消息后退出")
	agentCmd.Flags().StringVarP(&agentSession, "session", "s", "cli:default", "指定 Session Key，保留上下文对话")
	rootCmd.AddCommand(agentCmd)
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

	err := os.MkdirAll("./config", 0755)
	if err != nil {
		err = fmt.Errorf("创建配置目录失败(./config): %w", err)
		logger.L().Error().Err(err).Msg("")
		return err
	}

	file, err := os.Create("./config/config.json")
	if err != nil {
		err = fmt.Errorf("创建配置文件失败(./config/config.json): %w", err)
		logger.L().Error().Err(err).Msg("")
		return err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(cfg); err != nil {
		err = fmt.Errorf("保存配置失败(./config/config.json): %w", err)
		logger.L().Error().Err(err).Msg("")
		return err
	}

	fmt.Println("配置已成功保存至 ./config/config.json")
	logger.L().Debug().Str("action", "init").Msg("初始化配置完成")
	return nil
}

func runAgent(cmd *cobra.Command, args []string) error {
	logger.L().Debug().Msg("加载系统配置...")
	cfg, err := config.Load("./config/config.json")
	if err != nil {
		return err // 底层已经包装，直接透传
	}

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
	channelManager.Register(cliChan)

	// 4. 加载 Gateway 配置
	logger.L().Debug().Msg("加载 Gateway 配置...")
	gwCfg, err := gateway.LoadConfig("./gateway/example_gateway.yaml")
	if err != nil {
		return err // 底层已经包装，直接透传
	}

	// 5. 初始化 Gateway
	logger.L().Debug().Msg("初始化 Gateway...")
	gw := gateway.NewGateway(gwCfg, msgBus, channelManager)

	// 6. 创建并注册 Agent
	logger.L().Debug().Msg("创建并注册 Agent...")
	agentCfg := agent.AgentConfig{
		Name:       "main",
		LLM:        cfg.LLMConfig,
		MaxHistory: 5,
		Tools:      cfg.Tools,
	}
	mainAgent := agent.NewAgent(agentCfg, msgBus)
	mainAgent.RegisterTool(tools.NewFileSystemTool())
	mainAgent.RegisterTool(tools.NewShellTool())

	if cfg.Tools.WebSearch.Enabled && cfg.Tools.WebSearch.APIKey != "" {
		mainAgent.RegisterTool(tools.NewWebSearchTool(cfg.Tools.WebSearch.APIKey))
	}
	if cfg.Tools.UrlParse.Enabled {
		mainAgent.RegisterTool(tools.NewUrlParseTool())
	}

	if err := mainAgent.Start(ctx); err != nil {
		return err
	}

	if err := gw.RegisterAgent("main", mainAgent, agentCfg); err != nil {
		return err
	}

	// 7. 启动 Gateway 和 Channel
	logger.L().Debug().Msg("启动 Gateway 和 Channel...")
	if err := gw.Start(ctx); err != nil {
		return err
	}

	if err := channelManager.Start(ctx); err != nil {
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

	fmt.Println("Shutting down...")
	gw.Stop()
	channelManager.Stop()
	mainAgent.Stop()

	return nil
}
