package agent

import (
	"os"
	"testing"
	"time"
)

func TestSessionAnalyzer_AnalyzeAgent(t *testing.T) {
	// 创建临时目录
	tempDir, err := os.MkdirTemp("", "analyzer_test_*")
	if err != nil {
		t.Fatalf("创建临时目录失败: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// 创建测试数据
	agentName := "test_agent"
	channelName := "cli"
	chatID := "test_chat"

	store := NewSessionStore(tempDir)
	messages := []AgentMessage{
		{
			Role:      RoleUser,
			Timestamp: time.Now().UnixMilli(),
			Content:   []ContentBlock{TextContent{Text: "Hello"}},
		},
		{
			Role:      RoleAssistant,
			Timestamp: time.Now().UnixMilli(),
			Content:   []ContentBlock{TextContent{Text: "Hi!"}},
		},
	}

	err = store.SaveSession(agentName, channelName, chatID, messages)
	if err != nil {
		t.Fatalf("保存 session 失败: %v", err)
	}

	// 创建分析器
	analyzer := NewSessionAnalyzer(tempDir)

	// 分析 agent
	infos, err := analyzer.AnalyzeAgent(agentName)
	if err != nil {
		t.Fatalf("分析 agent 失败: %v", err)
	}

	if len(infos) != 1 {
		t.Errorf("期望找到 1 个 session，实际找到 %d 个", len(infos))
	}

	info := infos[0]
	if info.AgentName != agentName {
		t.Errorf("Agent 名称不匹配: got %s, want %s", info.AgentName, agentName)
	}

	if info.ChannelName != channelName {
		t.Errorf("Channel 名称不匹配: got %s, want %s", info.ChannelName, channelName)
	}

	if info.MessageCount != len(messages) {
		t.Errorf("消息数量不匹配: got %d, want %d", info.MessageCount, len(messages))
	}
}

func TestSessionAnalyzer_AnalyzeAll(t *testing.T) {
	// 创建临时目录
	tempDir, err := os.MkdirTemp("", "analyzer_test_*")
	if err != nil {
		t.Fatalf("创建临时目录失败: %v", err)
	}
	defer os.RemoveAll(tempDir)

	store := NewSessionStore(tempDir)

	// 创建多个 agent 的 session
	agents := []struct {
		name    string
		channel string
		chatID  string
	}{
		{"agent1", "cli", "chat1"},
		{"agent1", "cli", "chat2"},
		{"agent2", "weixin", "wx123"},
	}

	for _, a := range agents {
		messages := []AgentMessage{
			{
				Role:      RoleUser,
				Timestamp: time.Now().UnixMilli(),
				Content:   []ContentBlock{TextContent{Text: "Hello"}},
			},
		}
		err := store.SaveSession(a.name, a.channel, a.chatID, messages)
		if err != nil {
			t.Fatalf("保存 session 失败: %v", err)
		}
	}

	// 创建分析器
	analyzer := NewSessionAnalyzer(tempDir)

	// 分析所有 sessions
	infos, err := analyzer.AnalyzeAll()
	if err != nil {
		t.Fatalf("分析所有 sessions 失败: %v", err)
	}

	if len(infos) != len(agents) {
		t.Errorf("期望找到 %d 个 session，实际找到 %d 个", len(agents), len(infos))
	}
}

func TestPrintSessionInfo(t *testing.T) {
	// 这个测试主要确保函数不会 panic
	infos := []SessionInfo{
		{
			AgentName:    "test_agent",
			ChannelName:  "cli",
			ChatID:       "test_chat",
			StoragePath:  "/tmp/test",
			CreatedAt:    time.Now(),
			UpdatedAt:    time.Now(),
			MessageCount: 10,
			TokenCount:   100,
		},
	}

	// 调用函数，确保不会 panic
	PrintSessionInfo(infos)

	// 测试空列表
	PrintSessionInfo([]SessionInfo{})
}
