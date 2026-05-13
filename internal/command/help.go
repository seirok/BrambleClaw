package command

import (
	"brambleclaw/internal/bus"
	"brambleclaw/internal/interfaces"
	"context"
	"fmt"
	"strings"
)

// HelpCommand lists available commands and skills
type HelpCommand struct{}

// Name returns command name
func (c *HelpCommand) Name() string { return "help" }

// Description returns command description
func (c *HelpCommand) Description() string { return "List all available commands and skills" }

// Usage returns usage info
func (c *HelpCommand) Usage() string { return "/help" }

// Execute runs the command
func (c *HelpCommand) Execute(ctx context.Context, agent interface{}, msg interface{}, args []string) error {
	type agentWithHelp interface {
		Commands() interfaces.Registry[interfaces.Command]
		Name() string
	}
	// 定义一个临时接口，使用一个通用的类型
	type agentWithSkills interface {
		ListUserInvocableSkills() interface{}
	}

	a, ok := agent.(agentWithHelp)
	if !ok {
		return fmt.Errorf("agent does not have required methods")
	}

	m, ok := msg.(*bus.InBoundMessage)
	if !ok {
		return fmt.Errorf("invalid message type")
	}

	var sb strings.Builder
	sb.WriteString("Available commands:\n")
	for _, cmd := range a.Commands().List(ctx) {
		sb.WriteString(fmt.Sprintf("  %-10s - %s\n", cmd.Usage(), cmd.Description()))
	}

	// 尝试获取技能列表
	if sa, ok := agent.(agentWithSkills); ok {
		skills := sa.ListUserInvocableSkills()
		if skills != nil {
			// 类型断言为具体类型
			if slice, ok := skills.([]struct{ Name, Description string }); ok {
				if len(slice) > 0 {
					sb.WriteString("\nAvailable skills:\n")
					for _, s := range slice {
						sb.WriteString(fmt.Sprintf("  /%-10s - %s\n", s.Name, s.Description))
					}
				}
			}
		}
	}

	return publishReply(ctx, agent, m, sb.String())
}
