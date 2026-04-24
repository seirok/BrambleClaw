package test

import (
	"brambleclaw/bus"
	"brambleclaw/channel"
	"brambleclaw/config"
	"context"
	"log"
	"sync"
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

	msgBus := bus.NewMessageBus(cfg.BusBufSize)

	channelManager := channel.NewManager(msgBus)

	cli := channel.NewCLIChannel(cliCfg, msgBus)

	channelManager.Register(cli)
	channelManager.Start(ctx)

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		processMessage(ctx, msgBus)
	}()
	go func() {
		defer wg.Done()
		channelManager.DispatchOutbound(ctx)
	}()
	wg.Wait()
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
