package main

import (
	"brambleclaw/bus"
	"brambleclaw/cli"
	"brambleclaw/logger"
	"context"
	"time"
)

// TIP <p>To run your code, right-click the code and select <b>Run</b>.</p> <p>Alternatively, click
// the <icon src="AllIcons.Actions.Execute"/> icon in the gutter and select the <b>Run</b> menu item from here.</p>
func main() {
	if err := cli.Execute(); err != nil {
		logger.L().Fatal().Err(err).Msg("CLI执行失败")
	}
}

func processMessage(ctx context.Context, msgBus *bus.MessageBus) {
	// 消息分发线程持续监听
	go msgBus.DistributeOutBoundMessage(ctx)
	for {
		logger.L().Debug().Msg("ProcessMessage")
		in_msg, err := msgBus.ConsumeInBoundMessage(ctx)
		if err != nil {
			logger.L().Error().Err(err).Msg("消费消息失败")
			continue
		}
		outMsg := &bus.OutBoundMessage{
			ChatID:     "125",
			OutChannel: in_msg.InChannel,
			Content:    "Echo: " + in_msg.Content,
			TimeStamp:  time.Now(),
		}
		if err := msgBus.PublishOutBoundMessage(ctx, outMsg); err != nil {
			logger.L().Error().Err(err).Msg("发布消息失败")
		}
	}
}
