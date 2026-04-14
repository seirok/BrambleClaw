package agent

import (
	"brambleclaw/config"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestSummarizer_SummarizeAndSave(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "agent_test_*")
	if err != nil {
		t.Fatalf("创建临时目录失败: %v", err)
	}
	defer os.RemoveAll(tmpDir) // 测试结束后清理

	config, _ := config.Load("../config/config.json")
	llmClient := NewLLMClient(config.LLMConfig)
	summarizerConfig := DefaultSummarizerConfig()
	summarizerConfig.MessageThreshold = 10
	summarizerConfig.Workspace = tmpDir
	summarizer := NewSummarizer(summarizerConfig, llmClient)

	ctx := context.Background()
	messages := []*AgentMessage{
		{
			Role: RoleUser,
			Content: []ContentBlock{
				TextContent{Text: "你好，请帮我写个脚本"},
			},
		},
		{
			Role: RoleAssistant,
			Content: []ContentBlock{
				TextContent{Text: "好的，请问是什么语言？"},
			},
		},
	}
	sessionKey := "agent::main::test::123"
	err = summarizer.SummarizeAndSave(ctx, sessionKey, messages)
	if err != nil {
		t.Fatalf("SummarizeAndSave 失败: %v", err)
	}

	// 验证文件是否生成
	safeKey := "agent_main_test_123" // 预期中的转换后的文件名
	expectedPath := filepath.Join(tmpDir, "memory", safeKey+".meta.json")

	if _, err := os.Stat(expectedPath); os.IsNotExist(err) {
		t.Fatalf("预期的元数据文件未生成: %s", expectedPath)
	}

	// 读取并验证文件内容
	data, err := os.ReadFile(expectedPath)
	if err != nil {
		t.Fatalf("读取生成的文件失败: %v", err)
	}

	var meta SessionMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		t.Fatalf("解析 JSON 失败: %v", err)
	}

	// 5. 断言字段准确性
	if meta.Key != sessionKey {
		t.Errorf("Key 不匹配: 期望 %s, 得到 %s", sessionKey, meta.Key)
	}
	if meta.Count != len(messages) {
		t.Errorf("Count 不匹配: 期望 %d, 得到 %d", len(messages), meta.Count)
	}
	if meta.Summary == "" {
		t.Error("Summary 不能为空")
	}
	if meta.CreatedAt.IsZero() || meta.UpdatedAt.IsZero() {
		t.Error("时间戳未正确设置")
	}

}
