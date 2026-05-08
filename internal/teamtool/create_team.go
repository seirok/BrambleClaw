package teamtool

import (
	"brambleclaw/internal/agent"
	"brambleclaw/internal/messages"
	"brambleclaw/internal/team"
	"brambleclaw/internal/tools"
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// CreateTeamTool 创建二级 Agent 团队并执行任务
type CreateTeamTool struct {
	parentAgent *agent.Agent
}

func NewCreateTeamTool(parent *agent.Agent) *CreateTeamTool {
	return &CreateTeamTool{parentAgent: parent}
}

func (t *CreateTeamTool) Name() string { return "create_team" }

func (t *CreateTeamTool) Description() string {
	return "Create a team of specialized sub-agents to collaboratively solve a complex task. " +
		"Each participant has its own role, system prompt, and subset of tools."
}

func (t *CreateTeamTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"task": map[string]interface{}{
				"type":        "string",
				"description": "The task description for the team to solve",
			},
			"participants": map[string]interface{}{
				"type":        "array",
				"description": "List of team participants",
				"items": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"name": map[string]interface{}{
							"type":        "string",
							"description": "Participant name (unique within team)",
						},
						"description": map[string]interface{}{
							"type":        "string",
							"description": "Brief description of participant's role",
						},
						"system_prompt": map[string]interface{}{
							"type":        "string",
							"description": "Custom system prompt for this participant",
						},
						"tools": map[string]interface{}{
							"type":        "array",
							"items":       map[string]interface{}{"type": "string"},
							"description": "Names of tools available to this participant (subset of parent agent's tools)",
						},
					},
					"required": []string{"name", "description"},
				},
			},
			"team_type": map[string]interface{}{
				"type":        "string",
				"enum":        []string{"round_robin"},
				"description": "Team coordination type (default: round_robin)",
			},
			"max_turns": map[string]interface{}{
				"type":        "integer",
				"description": "Maximum number of message turns before termination (default: 10)",
			},
		},
		"required": []string{"task", "participants"},
	}
}

type createTeamArgs struct {
	Task         string           `json:"task"`
	Participants []participantArg `json:"participants"`
	TeamType     string           `json:"team_type"`
	MaxTurns     int              `json:"max_turns"`
}

type participantArg struct {
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	SystemPrompt string   `json:"system_prompt"`
	Tools        []string `json:"tools"`
}

func (t *CreateTeamTool) Execute(ctx context.Context, args string) (interface{}, error) {
	var req createTeamArgs
	if err := json.Unmarshal([]byte(args), &req); err != nil {
		return nil, fmt.Errorf("create_team: invalid arguments: %w", err)
	}

	if len(req.Participants) == 0 {
		return nil, fmt.Errorf("create_team: at least one participant required")
	}
	if req.MaxTurns <= 0 {
		req.MaxTurns = 10
	}
	if req.TeamType == "" {
		req.TeamType = "round_robin"
	}

	llmClient := t.parentAgent.Orchestrator().LLM()
	parentTools := t.parentAgent.Tools()

	participants := make([]agent.ChatAgent, 0, len(req.Participants))
	for _, p := range req.Participants {
		sub, err := agent.NewSubAgent(
			p.Name, p.Description, p.SystemPrompt,
			llmClient, parentTools, p.Tools,
		)
		if err != nil {
			return nil, fmt.Errorf("create_team: failed to create sub-agent %q: %w", p.Name, err)
		}
		participants = append(participants, sub)
	}

	groupChat, err := team.NewRoundRobinGroupChat("team", participants, req.MaxTurns)
	if err != nil {
		return nil, fmt.Errorf("create_team: failed to create team: %w", err)
	}

	taskMsg := messages.NewTextMessage("user", req.Task)

	result, err := groupChat.Run(ctx, taskMsg)
	if err != nil {
		return nil, fmt.Errorf("create_team: team execution failed: %w", err)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Team completed with %d messages:\n\n", len(result.Messages)))
	for i, msg := range result.Messages {
		sb.WriteString(fmt.Sprintf("[%d] %s: %s\n", i+1, msg.GetSource(), msg.ToText()))
	}

	return sb.String(), nil
}

// 编译时检查：确保 CreateTeamTool 实现了 Tool 接口
var _ tools.Tool = (*CreateTeamTool)(nil)
