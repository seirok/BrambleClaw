package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// SessionMeta 会话元数据结构，对应 .meta.json 文件
type SessionMeta struct {
	Key       string    `json:"key"`
	Summary   string    `json:"summary"`
	Skip      int       `json:"skip"`
	Count     int       `json:"count"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// SummarizerConfig 会话摘要配置
type SummarizerConfig struct {
	MessageThreshold      int
	TokenPercentThreshold int
	ContextWindow         int
	Workspace             string // 新增：基础工作目录
}

func DefaultSummarizerConfig() SummarizerConfig {
	return SummarizerConfig{
		MessageThreshold:      20,
		TokenPercentThreshold: 75,
		ContextWindow:         8192,
		Workspace:             "./workspace", // 默认路径
	}
}

// Summarizer 会话摘要器
type Summarizer struct {
	config SummarizerConfig
	llm    *LLMClient
}

func NewSummarizer(config SummarizerConfig, llm *LLMClient) *Summarizer {
	return &Summarizer{
		config: config,
		llm:    llm,
	}
}

// ShouldSummarize 保持不变... (略)

// SummarizeAndSave 生成会话摘要并持久化到本地文件
// sessionKey: 传入原始的 Session Key
// messages: 当前全量的历史消息列表
func (s *Summarizer) SummarizeAndSave(ctx context.Context, sessionKey string, messages []*AgentMessage) error {
	if len(messages) == 0 {
		return fmt.Errorf("没有消息需要摘要")
	}

	// 1. 调用 LLM 生成摘要内容
	summaryText, err := s.generateSummary(ctx, messages)
	if err != nil {
		return err
	}

	// 2. 构造 Meta 数据
	// 注意：skip 的逻辑通常是 MessageThreshold，即本次被压缩掉的消息数
	meta := SessionMeta{
		Key:       sessionKey,
		Summary:   summaryText,
		Skip:      s.config.MessageThreshold,
		Count:     len(messages),
		UpdatedAt: time.Now(),
	}

	// 3. 处理文件路径
	// 路径示例: {workspace}/memory/{session_key}.meta.json
	// 注意：如果 sessionKey 包含特殊字符（如冒号），在某些系统下需要做转义处理
	safeKey := strings.ReplaceAll(sessionKey, "::", "_")
	dirPath := filepath.Join(s.config.Workspace, "memory")
	filePath := filepath.Join(dirPath, fmt.Sprintf("%s.meta.json", safeKey))

	// 确保目录存在
	if err := os.MkdirAll(dirPath, 0755); err != nil {
		return fmt.Errorf("创建目录失败: %w", err)
	}

	// 4. 如果文件已存在，尝试读取旧的 CreatedAt
	if oldData, err := os.ReadFile(filePath); err == nil {
		var oldMeta SessionMeta
		if err := json.Unmarshal(oldData, &oldMeta); err == nil {
			meta.CreatedAt = oldMeta.CreatedAt
		}
	}
	if meta.CreatedAt.IsZero() {
		meta.CreatedAt = time.Now()
	}

	// 5. 写入 JSON 文件
	fileData, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化 JSON 失败: %w", err)
	}

	return os.WriteFile(filePath, fileData, 0644)
}

// generateSummary 内部私有方法，专门负责与 LLM 交互
func (s *Summarizer) generateSummary(ctx context.Context, messages []*AgentMessage) (string, error) {
	var prompt strings.Builder
	prompt.WriteString("请对以下对话进行摘要。保留关键信息、决策、行动项和重要上下文。\n\n对话内容：\n")

	for _, msg := range messages {
		role := "用户"
		if msg.Role == RoleAssistant {
			role = "助手"
		}
		for _, content := range msg.Content {
			if text, ok := content.(TextContent); ok && text.Text != "" {
				prompt.WriteString(fmt.Sprintf("%s: %s\n", role, text.Text))
			}
		}
	}

	req := ChatCompletionRequest{
		Model: s.llm.config.Model,
		Messages: []ChatMsg{
			{
				Role:    RoleUser,
				Content: prompt.String(),
			},
		},
	}

	resp, err := s.llm.Chat(req)
	if err != nil {
		return "", fmt.Errorf("生成摘要失败: %w", err)
	}

	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("LLM返回空结果")
	}

	return resp.Choices[0].Message.Content, nil
}
