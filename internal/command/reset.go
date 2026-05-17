package command

import (
	"context"
	"fmt"
	util "neoclaw/internal"
	"neoclaw/internal/bus"
)

// ResetCommand resets the current session, clearing messages but keeping the same session key
type ResetCommand struct{}

// Name returns command name
func (c *ResetCommand) Name() string { return "reset" }

// Description returns command description
func (c *ResetCommand) Description() string {
	return "Reset current session (clear messages, keep session key)"
}

// Usage returns usage info
func (c *ResetCommand) Usage() string { return "/reset" }

// Execute runs the command
func (c *ResetCommand) Execute(ctx context.Context, agent interface{}, msg interface{}, args []string) error {
	type agentWithReset interface {
		ResetSession(sessionKey string) error
		Name() string
	}
	a, ok := agent.(agentWithReset)
	if !ok {
		return fmt.Errorf("agent does not support ResetSession")
	}

	m, ok := msg.(*bus.InBoundMessage)
	if !ok {
		return fmt.Errorf("invalid message type")
	}

	sessKey := util.BuildSessionKey(a.Name(), m.InChannel, m.ChatID)
	err := a.ResetSession(sessKey)

	var reply string
	if err != nil {
		reply = fmt.Sprintf("Failed to reset session: %v", err)
	} else {
		reply = "Session reset, all messages cleared."
	}

	return publishReply(ctx, agent, m, reply)
}
