package teamtool

import (
	"context"
	"encoding/json"
	"fmt"
	"neoclaw/internal/agent"
	"neoclaw/internal/interfaces"
	"neoclaw/internal/logger"
	"neoclaw/internal/messages"
	"neoclaw/internal/team"
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
				"enum":        []string{"round_robin", "selector"},
				"description": "Team coordination type (default: round_robin)",
			},
			"max_turns": map[string]interface{}{
				"type":        "integer",
				"description": "Maximum number of message turns before termination (default: 10)",
			},

			"error_policy": map[string]interface{}{
				"type":        "string",
				"enum":        []string{"terminate", "skip"},
				"description": "Error handling policy: terminate (stop team on error, default) or skip (skip failed member and continue)",
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
	ErrorPolicy  string           `json:"error_policy"`
}

type participantArg struct {
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	SystemPrompt string   `json:"system_prompt"`
	Tools        []string `json:"tools"`
}

func (t *CreateTeamTool) Execute(ctx context.Context, args string) (interface{}, error) {
	logger.L().Debug().Msg("using CreateTeam Tool...")
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

	// 解析错误策略
	var errorPolicy team.ErrorPolicy
	switch req.ErrorPolicy {
	case "skip":
		errorPolicy = team.ErrorPolicySkip
	case "terminate", "":
		errorPolicy = team.ErrorPolicyTerminate
	default:
		return nil, fmt.Errorf("create_team: invalid error_policy %q, must be 'terminate' or 'skip'", req.ErrorPolicy)
	}

	llmClient := t.parentAgent.Orchestrator().LLM()
	parentTools := t.parentAgent.Tools()
	auditLogger := t.parentAgent.Orchestrator().AuditLogger()

	participants := make([]agent.ChatAgent, 0, len(req.Participants))
	for _, p := range req.Participants {
		sub, err := agent.NewSubAgent(
			p.Name, p.Description, p.SystemPrompt,
			llmClient, parentTools, p.Tools, auditLogger,
		)
		if err != nil {
			return nil, fmt.Errorf("create_team: failed to create sub-agent %q: %w", p.Name, err)
		}
		logger.L().Debug().Str("sub-agent", p.Name).Msg("created sub-agent")
		participants = append(participants, sub)
	}

	var groupChat team.Team
	var err error
	switch req.TeamType {
	case "round_robin":
		groupChat, err = team.NewRoundRobinGroupChat("team", participants, req.MaxTurns, errorPolicy)
	case "selector":
		groupChat, err = team.NewSelectorGroupChat("team", participants, req.MaxTurns, errorPolicy, llmClient, team.DefaultMaxHistory)
	default:
		return nil, fmt.Errorf("create_team: invalid team_type %q, must be 'round_robin' or 'selector'", req.TeamType)
	}
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
		if messages.IsErrorMessage(msg) {
			sb.WriteString(fmt.Sprintf("[%d] ERROR - %s: %s\n", i+1, msg.GetSource(), messages.GetErrorDetail(msg)))
		} else {
			sb.WriteString(fmt.Sprintf("[%d] %s: %s\n", i+1, msg.GetSource(), msg.ToText()))
		}
	}

	if result.Error != "" {
		sb.WriteString(fmt.Sprintf("\nTeam encountered errors: %s\n", result.Error))
	}

	return sb.String(), nil
}

// 编译时检查：确保 CreateTeamTool 实现了 Tool 接口
var _ interfaces.Tool = (*CreateTeamTool)(nil)
