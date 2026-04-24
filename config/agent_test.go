package config

import (
	"testing"
)

func TestAgentsConfig_GetAgent(t *testing.T) {
	agents := &AgentsConfig{
		Agents: []AgentConfig{
			{
				Name:        "main",
				Description: "Main agent",
				Workspace:   "/workspace/main",
				MaxHistory:  10,
				Enabled:     true,
			},
			{
				Name:        "secondary",
				Description: "Secondary agent",
				Workspace:   "/workspace/secondary",
				MaxHistory:  5,
				Enabled:     false,
			},
		},
	}

	tests := []struct {
		name       string
		agentName  string
		wantFound  bool
		wantConfig *AgentConfig
	}{
		{
			name:      "existing agent main",
			agentName: "main",
			wantFound: true,
			wantConfig: &AgentConfig{
				Name:        "main",
				Description: "Main agent",
				Workspace:   "/workspace/main",
				MaxHistory:  10,
				Enabled:     true,
			},
		},
		{
			name:       "non-existing agent",
			agentName:  "nonexistent",
			wantFound:  false,
			wantConfig: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, found := agents.GetAgent(tt.agentName)
			if found != tt.wantFound {
				t.Errorf("GetAgent() found = %v, want %v", found, tt.wantFound)
				return
			}
			if tt.wantFound {
				if got.Name != tt.wantConfig.Name {
					t.Errorf("GetAgent() Name = %v, want %v", got.Name, tt.wantConfig.Name)
				}
				if got.Description != tt.wantConfig.Description {
					t.Errorf("GetAgent() Description = %v, want %v", got.Description, tt.wantConfig.Description)
				}
			}
		})
	}
}

func TestAgentsConfig_AddOrUpdateAgent(t *testing.T) {
	agents := &AgentsConfig{
		Agents: []AgentConfig{
			{
				Name:        "main",
				Description: "Main agent",
				MaxHistory:  10,
			},
		},
	}

	// 测试添加新 agent
	newAgent := AgentConfig{
		Name:        "secondary",
		Description: "Secondary agent",
		MaxHistory:  5,
	}

	agents.AddOrUpdateAgent(newAgent)

	if len(agents.Agents) != 2 {
		t.Errorf("添加后 agent 数量 = %d, want 2", len(agents.Agents))
	}

	found, exists := agents.GetAgent("secondary")
	if !exists {
		t.Error("新添加的 agent 应该存在")
	}
	if found.MaxHistory != 5 {
		t.Errorf("新 agent 的 MaxHistory = %d, want 5", found.MaxHistory)
	}

	// 测试更新现有 agent
	updatedAgent := AgentConfig{
		Name:        "main",
		Description: "Updated main agent",
		MaxHistory:  20,
	}

	agents.AddOrUpdateAgent(updatedAgent)

	if len(agents.Agents) != 2 {
		t.Errorf("更新后 agent 数量应该仍然是 %d, want 2", len(agents.Agents))
	}

	found, exists = agents.GetAgent("main")
	if !exists {
		t.Fatal("更新后的 agent 应该存在")
	}
	if found.Description != "Updated main agent" {
		t.Errorf("agent 的 Description = %s, want 'Updated main agent'", found.Description)
	}
	if found.MaxHistory != 20 {
		t.Errorf("agent 的 MaxHistory = %d, want 20", found.MaxHistory)
	}
}

func TestAgentsConfig_RemoveAgent(t *testing.T) {
	agents := &AgentsConfig{
		Agents: []AgentConfig{
			{Name: "agent1"},
			{Name: "agent2"},
			{Name: "agent3"},
		},
	}

	// 删除存在的 agent
	removed := agents.RemoveAgent("agent2")
	if !removed {
		t.Error("删除存在的 agent 应该返回 true")
	}

	if len(agents.Agents) != 2 {
		t.Errorf("删除后 agent 数量 = %d, want 2", len(agents.Agents))
	}

	_, exists := agents.GetAgent("agent2")
	if exists {
		t.Error("删除的 agent 不应该存在")
	}

	// 删除不存在的 agent
	removed = agents.RemoveAgent("nonexistent")
	if removed {
		t.Error("删除不存在的 agent 应该返回 false")
	}

	if len(agents.Agents) != 2 {
		t.Errorf("删除不存在的 agent 后数量应该仍然是 %d, want 2", len(agents.Agents))
	}
}
