package command

import (
	"context"
	"fmt"
	"neoclaw/internal/bus"
	"neoclaw/internal/skill"
	"strings"
)

// SkillsCommand 列出所有可用技能及其详情
type SkillsCommand struct{}

// Name returns command name
func (c *SkillsCommand) Name() string { return "skills" }

// Description returns command description
func (c *SkillsCommand) Description() string { return "List all available skills with details" }

// Usage returns usage info
func (c *SkillsCommand) Usage() string { return "/skills" }

// Execute runs the command
func (c *SkillsCommand) Execute(ctx context.Context, agent interface{}, msg interface{}, args []string) error {
	type agentWithSkills interface {
		ListAllSkills() []skill.SkillInfo
		Name() string
	}

	a, ok := agent.(agentWithSkills)
	if !ok {
		return fmt.Errorf("agent does not support skills listing")
	}

	m, ok := msg.(*bus.InBoundMessage)
	if !ok {
		return fmt.Errorf("invalid message type")
	}

	var sb strings.Builder
	skills := a.ListAllSkills()

	if len(skills) == 0 {
		sb.WriteString("No skills available.")
		return publishReply(ctx, agent, m, sb.String())
	}

	sb.WriteString(fmt.Sprintf("Available skills (%d):\n", len(skills)))
	for _, s := range skills {
		sb.WriteString(fmt.Sprintf("\n  - %s (%s)\n", s.Name, s.Scope))
		if s.Description != "" {
			sb.WriteString(fmt.Sprintf("    %s\n", s.Description))
		}
		// 调用方式
		var invocations []string
		if s.UserInvocable {
			invocations = append(invocations, "/"+s.Name)
		}
		if !s.DisableModelInvoke {
			invocations = append(invocations, "model-invoke")
		}
		if len(invocations) > 0 {
			sb.WriteString(fmt.Sprintf("    Invoke: %s\n", strings.Join(invocations, ", ")))
		}
		// 参数
		if len(s.Arguments) > 0 {
			sb.WriteString("    Args:\n")
			for _, arg := range s.Arguments {
				var reqStr string
				if arg.Required {
					reqStr = " (required)"
				}
				var defaultStr string
				if arg.Default != "" {
					defaultStr = fmt.Sprintf(" (default: %q)", arg.Default)
				}
				sb.WriteString(fmt.Sprintf("      - %s%s%s: %s\n", arg.Name, reqStr, defaultStr, arg.Description))
			}
		}
	}

	return publishReply(ctx, agent, m, sb.String())
}
