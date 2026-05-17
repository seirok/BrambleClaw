package skill

import (
	"context"
	"neoclaw/internal/logger"
)

// SkillCommand adapts a skill to interfaces.Command
type SkillCommand struct {
	skillManager *SkillManager
	skillName    string
	skillMeta    *SkillMeta
}

// NewSkillCommand creates a SkillCommand
func NewSkillCommand(sm *SkillManager, meta *SkillMeta) *SkillCommand {
	return &SkillCommand{
		skillManager: sm,
		skillName:    meta.Name,
		skillMeta:    meta,
	}
}

// Name returns skill name
func (c *SkillCommand) Name() string {
	return c.skillName
}

// Description returns skill description
func (c *SkillCommand) Description() string {
	return c.skillMeta.Description
}

// Usage returns usage info
func (c *SkillCommand) Usage() string {
	return "/" + c.skillName
}

// Execute runs the skill command
func (c *SkillCommand) Execute(ctx context.Context, agent interface{}, msg interface{}, args []string) error {
	skillArgs := SkillInvocationArgs{
		Positional: args,
		Source:     "command",
	}

	content, err := c.skillManager.Execute(ctx, c.skillName, skillArgs)
	if err != nil {
		logger.L().Error().Err(err).Str("skill", c.skillName).Msg("Failed to execute skill")
		return err
	}

	logger.L().Debug().Str("skill", c.skillName).Str("content", content[:min(500, len(content))]).Msg("Skill executed")
	return nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
