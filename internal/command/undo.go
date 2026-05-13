package command

import (
	util "brambleclaw/internal"
	"brambleclaw/internal/bus"
	"context"
	"fmt"
)

// UndoCommand removes the last user-assistant message pair
type UndoCommand struct{}

// Name returns command name
func (c *UndoCommand) Name() string { return "undo" }

// Description returns command description
func (c *UndoCommand) Description() string { return "Undo the last conversation round" }

// Usage returns usage info
func (c *UndoCommand) Usage() string { return "/undo" }

// Execute runs the command
func (c *UndoCommand) Execute(ctx context.Context, agent interface{}, msg interface{}, args []string) error {
	type agentWithUndo interface {
		UndoLastRound(sessionKey string) (int, error)
		Name() string
	}
	a, ok := agent.(agentWithUndo)
	if !ok {
		return fmt.Errorf("agent does not support UndoLastRound")
	}

	m, ok := msg.(*bus.InBoundMessage)
	if !ok {
		return fmt.Errorf("invalid message type")
	}

	sessKey := util.BuildSessionKey(a.Name(), m.InChannel, m.ChatID)
	count, err := a.UndoLastRound(sessKey)

	var reply string
	if err != nil {
		reply = fmt.Sprintf("Failed to undo: %v", err)
	} else if count == 0 {
		reply = "No conversation to undo."
	} else {
		reply = fmt.Sprintf("Undo complete (%d messages removed).", count)
	}

	return publishReply(ctx, agent, m, reply)
}
