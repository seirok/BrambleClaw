package main

import (
	"context"
	"log"
	"miniGoClaw/bus"
	"miniGoClaw/cli"
	"time"
)

// TIP <p>To run your code, right-click the code and select <b>Run</b>.</p> <p>Alternatively, click
// the <icon src="AllIcons.Actions.Execute"/> icon in the gutter and select the <b>Run</b> menu item from here.</p>
func main() {
	if err := cli.Execute(); err != nil {
		log.Fatal(err)
	}
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
