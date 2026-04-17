package test

import (
	"brambleclaw/bus"
	"brambleclaw/config"
	"context"
	"log"
	"sync"
	"testing"
	"time"
)

func TestLab1(t *testing.T) {
	cfg, _ := config.Load("../config/config.json")
	msgBus := bus.NewMessageBus(cfg.BusBufSize)
	ctx := context.Background()
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		simulateMessage(msgBus)
	}()
	go func() {
		defer wg.Done()
		processMessage(ctx, msgBus)
	}()
	wg.Wait()
}

func simulateMessage(msgBus *bus.MessageBus) {
	// 发布消息到InBound总线上
	inMsg := &bus.InBoundMessage{
		SenderID:  "userA",
		ChatID:    "1",
		InChannel: "QQ",
		Content:   "Hello from QQ channel",
		TimeStamp: time.Now(),
	}
	msgBus.PublishInBoundMessage(context.Background(), inMsg)

	// 订阅消息
	// 之后消息就会存放在MessageSubscription结构体中
	sub := msgBus.Subscribe()
	select {
	case msg := <-sub.Channel:
		log.Printf("Received message: %s", msg.Content)

	case <-time.After(5 * time.Second):
		log.Println("Timeout waiting for response")
		return
	}

}
