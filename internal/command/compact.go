package command

import (
	util "brambleclaw/internal"
	"brambleclaw/internal/bus"
	"context"
	"fmt"
)

// CompactCommand manually triggers context compression
type CompactCommand struct{}

// Name returns command name
func (c *CompactCommand) Name() string { return "compact" }

// Description returns command description
func (c *CompactCommand) Description() string { return "Manually trigger context compression" }

// Usage returns usage info
func (c *CompactCommand) Usage() string { return "/compact" }

// Execute runs the command
func (c *CompactCommand) Execute(ctx context.Context, agent interface{}, msg interface{}, args []string) error {
	type agentWithCompact interface {
		ForceCompactSession(ctx context.Context, sessionKey string) (int, error)
		Name() string
	}
	a, ok := agent.(agentWithCompact)
	if !ok {
		return fmt.Errorf("agent does not support ForceCompactSession")
	}

	m, ok := msg.(*bus.InBoundMessage)
	if !ok {
		return fmt.Errorf("invalid message type")
	}

	sessKey := util.BuildSessionKey(a.Name(), m.InChannel, m.ChatID)
	count, err := a.ForceCompactSession(ctx, sessKey)

	var reply string
	if err != nil {
		reply = fmt.Sprintf("Failed to compact context: %v", err)
	} else if count == 0 {
		reply = "No messages to compact."
	} else {
		reply = fmt.Sprintf("Compacted %d messages, context size reduced.", count)
	}

	return publishReply(ctx, agent, m, reply)
}
