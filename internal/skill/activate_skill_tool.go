package skill

import (
	"context"
	"encoding/json"
	"fmt"
)

// Tool defines the same interface as internal/tools.Tool (avoids import cycle)
type Tool interface {
	Name() string
	Description() string
	Execute(ctx context.Context, args string) (interface{}, error)
	Parameters() map[string]interface{}
}

// ActivateSkillTool is the tool for activating skills
type ActivateSkillTool struct {
	skillManager *SkillManager
}

// NewActivateSkillTool creates an ActivateSkillTool
func NewActivateSkillTool(sm *SkillManager) *ActivateSkillTool {
	return &ActivateSkillTool{
		skillManager: sm,
	}
}

// Name returns tool name
func (t *ActivateSkillTool) Name() string {
	return "activate_skill"
}

// Description returns tool description
func (t *ActivateSkillTool) Description() string {
	return "Activate a skill by name and get its content with variable substitution applied"
}

// Parameters returns tool parameters schema
func (t *ActivateSkillTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"name": map[string]interface{}{
				"type":        "string",
				"description": "The name of the skill to activate",
			},
			"arguments": map[string]interface{}{
				"type":        "array",
				"items":       map[string]interface{}{"type": "string"},
				"description": "Positional arguments for the skill",
			},
			"named": map[string]interface{}{
				"type":        "object",
				"description": "Named arguments for the skill",
			},
		},
		"required": []string{"name"},
	}
}

// Execute runs the tool
func (t *ActivateSkillTool) Execute(ctx context.Context, args string) (interface{}, error) {
	type Input struct {
		Name      string            `json:"name"`
		Arguments []string          `json:"arguments"`
		Named     map[string]string `json:"named"`
	}
	var input Input
	if err := json.Unmarshal([]byte(args), &input); err != nil {
		return nil, err
	}

	skillArgs := SkillInvocationArgs{
		Positional: input.Arguments,
		Named:      input.Named,
		Source:     "model",
	}

	content, err := t.skillManager.Execute(ctx, input.Name, skillArgs)
	if err != nil {
		if err == ErrInvocationBlocked {
			return fmt.Sprintf("Skill %q cannot be auto-activated. Use /%s instead.", input.Name, input.Name), nil
		}
		return nil, err
	}

	return content, nil
}
