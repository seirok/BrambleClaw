package test

import (
	"brambleclaw/bus"
	"brambleclaw/channel"
	"brambleclaw/config"
	"context"
	"sync"
	"testing"
)

func TestLab2(t *testing.T) {
	cfg, _ := config.Load("../config/config.json")
	msgBus := bus.NewMessageBus(cfg.BusBufSize)
	base_cfg := &channel.BaseChannelConfig{
		Enabled:    cfg.Channels.CLI.Enabled,
		AllowedIDs: cfg.Channels.CLI.AllowedIDs,
	}
	cli := channel.NewCLIChannel(base_cfg, msgBus)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel() // ?
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		cli.Start(ctx)
	}()
	go func() {
		defer wg.Done()
		processMessage(msgBus)
	}()
	wg.Wait()
}
