package command

import (
	"brambleclaw/internal/bus"
	"context"
	"fmt"
	"strings"
)

// ModelCommand 查看或切换 LLM 模型
type ModelCommand struct{}

// Name returns command name
func (c *ModelCommand) Name() string { return "model" }

// Description returns command description
func (c *ModelCommand) Description() string { return "View or switch LLM model" }

// Usage returns usage info
func (c *ModelCommand) Usage() string { return "/model [new-model]" }

// Execute runs the command
func (c *ModelCommand) Execute(ctx context.Context, agent interface{}, msg interface{}, args []string) error {
	type agentWithModel interface {
		CurrentModel() string
		SwitchModel(model string) error
		Name() string
	}

	a, ok := agent.(agentWithModel)
	if !ok {
		return fmt.Errorf("agent does not support model switching")
	}

	m, ok := msg.(*bus.InBoundMessage)
	if !ok {
		return fmt.Errorf("invalid message type")
	}

	if len(args) == 0 || strings.TrimSpace(args[0]) == "" {
		current := a.CurrentModel()
		var reply string
		if current == "" {
			reply = "No model configured."
		} else {
			reply = fmt.Sprintf("Current model: %s", current)
		}
		return publishReply(ctx, agent, m, reply)
	}

	newModel := strings.TrimSpace(args[0])
	oldModel := a.CurrentModel()
	if err := a.SwitchModel(newModel); err != nil {
		reply := fmt.Sprintf("Failed to switch model: %v", err)
		return publishReply(ctx, agent, m, reply)
	}

	var reply string
	if oldModel == "" {
		reply = fmt.Sprintf("Model set to: %s", newModel)
	} else {
		reply = fmt.Sprintf("Model switched from: %s → %s", oldModel, newModel)
	}
	return publishReply(ctx, agent, m, reply)
}