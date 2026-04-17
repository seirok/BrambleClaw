package agent

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSessionStore_SaveAndLoadSession(t *testing.T) {
	// 创建临时目录
	tempDir, err := os.MkdirTemp("", "session_test_*")
	if err != nil {
		t.Fatalf("创建临时目录失败: %v", err)
	}
	defer os.RemoveAll(tempDir)

	store := NewSessionStore(tempDir)

	// 创建测试消息
	messages := []AgentMessage{
		{
			Role:      RoleUser,
			Timestamp: time.Now().UnixMilli(),
		},
		{
			Role:      RoleAssistant,
			Timestamp: time.Now().UnixMilli(),
		},
	}

	// 添加内容到消息
	messages[0].Content = []ContentBlock{TextContent{Text: "Hello"}}
	messages[1].Content = []ContentBlock{TextContent{Text: "Hi there!"}}

	// 保存 session
	agentName := "test_agent"
	channelName := "cli"
	chatID := "test123"

	err = store.SaveSession(agentName, channelName, chatID, messages)
	if err != nil {
		t.Fatalf("保存 session 失败: %v", err)
	}

	// 加载 session
	loadedMessages, metadata, err := store.LoadSession(agentName, channelName, chatID)
	if err != nil {
		t.Fatalf("加载 session 失败: %v", err)
	}

	// 验证
	if len(loadedMessages) != len(messages) {
		t.Errorf("消息数量不匹配: got %d, want %d", len(loadedMessages), len(messages))
	}

	if metadata.AgentName != agentName {
		t.Errorf("Agent 名称不匹配: got %s, want %s", metadata.AgentName, agentName)
	}

	if metadata.MessageCount != len(messages) {
		t.Errorf("消息计数不匹配: got %d, want %d", metadata.MessageCount, len(messages))
	}

	// 验证文件是否存在
	memoryDir := store.GetMemoryDir(agentName)
	sessionFile := filepath.Join(memoryDir, store.BuildSessionFilename(agentName, channelName, chatID))
	if _, err := os.Stat(sessionFile); os.IsNotExist(err) {
		t.Errorf("session 文件不存在: %s", sessionFile)
	}
}

func TestSessionStore_LoadNonExistentSession(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "session_test_*")
	if err != nil {
		t.Fatalf("创建临时目录失败: %v", err)
	}
	defer os.RemoveAll(tempDir)

	store := NewSessionStore(tempDir)

	// 尝试加载不存在的 session
	messages, metadata, err := store.LoadSession("non_existent", "cli", "123")
	if err != nil {
		t.Fatalf("加载不存在的 session 应该返回空而不是错误: %v", err)
	}

	if len(messages) != 0 {
		t.Errorf("不存在的 session 应该返回空消息列表，got %d messages", len(messages))
	}

	if metadata == nil {
		t.Error("不存在的 session 应该返回默认元数据，而不是 nil")
	}
}

func TestSessionStore_ListSessions(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "session_test_*")
	if err != nil {
		t.Fatalf("创建临时目录失败: %v", err)
	}
	defer os.RemoveAll(tempDir)

	store := NewSessionStore(tempDir)

	agentName := "test_agent"

	// 创建几个 session
	sessions := []struct {
		channel string
		chatID  string
	}{
		{"cli", "chat1"},
		{"cli", "chat2"},
		{"weixin", "wx123"},
	}

	for _, s := range sessions {
		messages := []AgentMessage{
			{
				Role:      RoleUser,
				Timestamp: time.Now().UnixMilli(),
				Content:   []ContentBlock{TextContent{Text: "Test"}},
			},
		}
		err := store.SaveSession(agentName, s.channel, s.chatID, messages)
		if err != nil {
			t.Fatalf("保存 session 失败: %v", err)
		}
	}

	// 列出 sessions
	metadatas, err := store.ListSessions(agentName)
	if err != nil {
		t.Fatalf("列出 sessions 失败: %v", err)
	}

	if len(metadatas) != len(sessions) {
		t.Errorf("session 数量不匹配: got %d, want %d", len(metadatas), len(sessions))
	}
}

func TestSessionStore_DeleteSession(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "session_test_*")
	if err != nil {
		t.Fatalf("创建临时目录失败: %v", err)
	}
	defer os.RemoveAll(tempDir)

	store := NewSessionStore(tempDir)

	agentName := "test_agent"
	channelName := "cli"
	chatID := "test123"

	// 创建 session
	messages := []AgentMessage{
		{
			Role:      RoleUser,
			Timestamp: time.Now().UnixMilli(),
			Content:   []ContentBlock{TextContent{Text: "Test"}},
		},
	}

	err = store.SaveSession(agentName, channelName, chatID, messages)
	if err != nil {
		t.Fatalf("保存 session 失败: %v", err)
	}

	// 删除 session
	err = store.DeleteSession(agentName, channelName, chatID)
	if err != nil {
		t.Fatalf("删除 session 失败: %v", err)
	}

	// 验证文件已删除
	memoryDir := store.GetMemoryDir(agentName)
	sessionFile := filepath.Join(memoryDir, store.BuildSessionFilename(agentName, channelName, chatID))
	metaFile := filepath.Join(memoryDir, store.BuildMetadataFilename(agentName, channelName, chatID))

	if _, err := os.Stat(sessionFile); !os.IsNotExist(err) {
		t.Errorf("session 文件应该被删除: %s", sessionFile)
	}

	if _, err := os.Stat(metaFile); !os.IsNotExist(err) {
		t.Errorf("元数据文件应该被删除: %s", metaFile)
	}
}
