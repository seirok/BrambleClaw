package main

import (
	"context"
	"log"
	"miniGoClaw/agent"
	"miniGoClaw/bus"
	"miniGoClaw/channel"
	"miniGoClaw/config"
	"miniGoClaw/tools"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// TIP <p>To run your code, right-click the code and select <b>Run</b>.</p> <p>Alternatively, click
// the <icon src="AllIcons.Actions.Execute"/> icon in the gutter and select the <b>Run</b> menu item from here.</p>
func main() {

	// 创建，注册，启动Channel
	cfg, err := config.Load("./config/config.json")
	if err != nil {
		log.Fatal(err)
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cliCfg := &channel.BaseChannelConfig{
		Enabled:    cfg.Channels.CLI.Enabled,
		AllowedIDs: cfg.Channels.CLI.AllowedIDs,
	}

	// 创建消息总线
	msgBus := bus.NewMessageBus(cfg.BusBufSize)

	// 创建通道管理器
	channelManager := channel.NewManager(msgBus)

	// 创建 CLI 通道
	cli := channel.NewCLIChannel(cliCfg, msgBus)

	channelManager.Register(cli)
	channelManager.Start(ctx)

	// 创建基于DS Agent
	llmCfg := config.LLMConfig{
		APIKey:  "sk-fd2edcb9e8814784a1cf0a7ddbe082ee",
		BaseURL: "https://api.deepseek.com/v1/chat/completions",
		Model:   "deepseek-chat",
	}
	// 创建Agent
	agentCfg := agent.AgentConfig{
		Name:       "DeepSeek",
		LLM:        llmCfg,
		MaxHistory: 5,
		Tools:      cfg.Tools,
	}
	ds_agent := agent.NewAgent(agentCfg, msgBus)
	ds_agent.RegisterTool(tools.NewFileSystemTool())
	ds_agent.RegisterTool(tools.NewShellTool())

	if cfg.Tools.WebSearch.Enabled {
		if cfg.Tools.WebSearch.APIKey == "" {
			log.Println("WebSearch 工具已启用，但未配置 API Key。请在 config.json 中配置 tools.web_search.api_key。")
		} else {
			ds_agent.RegisterTool(tools.NewWebSearchTool(cfg.Tools.WebSearch.APIKey))
		}
	}

	agentManager := agent.NewAgentManager(msgBus)

	// 注册Agent
	agentManager.Register(agentCfg.Name, ds_agent)

	agentManager.Start(ctx)

	// 订阅出站消息并分发到对应Channel
	go channelManager.DispatchOutbound(ctx)

	//

	// 等待信号
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	log.Println("Shutting down...")
	agentManager.Stop()
	channelManager.Stop()

}

func processMessage(ctx context.Context, msgBus *bus.MessageBus) {
	// 消息分发线程持续监听
	go msgBus.DistributeOutBoundMessage(ctx)
	for {
		log.Println("ProcessMessage")
		in_msg, _ := msgBus.ConsumeInBoundMessage(ctx)
		outMsg := &bus.OutBoundMessage{
			ChatID:     "125",
			OutChannel: in_msg.InChannel,
			Content:    "Echo: " + in_msg.Content,
			TimeStamp:  time.Now(),
		}
		msgBus.PublishOutBoundMessage(ctx, outMsg)
	}
}
