package command

import (
	"context"
	"fmt"
	util "neoclaw/internal"
	"neoclaw/internal/bus"
	"neoclaw/internal/logger"
)

// ClearCommand clears the session by creating a new one
type ClearCommand struct{}

// Name returns command name
func (c *ClearCommand) Name() string { return "clear" }

// Description returns command description
func (c *ClearCommand) Description() string { return "Create new session (keep old session records)" }

// Usage returns usage info
func (c *ClearCommand) Usage() string { return "/clear" }

// Execute runs the command
func (c *ClearCommand) Execute(ctx context.Context, agent interface{}, msg interface{}, args []string) error {
	// Type assert agent to something that has ClearSession method and Name()
	type agentWithSession interface {
		ClearSession(sessionKey string) (int, error)
		Name() string
	}
	a, ok := agent.(agentWithSession)
	if !ok {
		return fmt.Errorf("agent does not support ClearSession")
	}

	// Type assert msg to bus.InBoundMessage
	m, ok := msg.(*bus.InBoundMessage)
	if !ok {
		return fmt.Errorf("invalid message type")
	}

	sessKey := util.BuildSessionKey(a.Name(), m.InChannel, m.ChatID)
	count, err := a.ClearSession(sessKey)

	var reply string
	if err != nil {
		reply = fmt.Sprintf("Failed to create new session: %v", err)
	} else {
		reply = fmt.Sprintf("Created new session, old session has %d messages kept.", count)
	}
	logger.L().Info().Str("command", "clear").Msg(reply)

	if m != nil {
		_ = publishReply(ctx, agent, m, reply)
	}

	return err
}
