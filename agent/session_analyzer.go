package agent

import (
	"brambleclaw/logger"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// SessionInfo session 信息（用于分析器输出）
type SessionInfo struct {
	AgentName    string
	ChannelName  string
	ChatID       string
	StoragePath  string
	CreatedAt    time.Time
	UpdatedAt    time.Time
	MessageCount int
	TokenCount   int
}

// SessionAnalyzer session 分析器
type SessionAnalyzer struct {
	store *SessionStore
}

// NewSessionAnalyzer 创建 session 分析器
func NewSessionAnalyzer(workspacePath string) *SessionAnalyzer {
	store := NewSessionStore(workspacePath)
	return &SessionAnalyzer{
		store: store,
	}
}

// AnalyzeAll 分析所有 agent 的所有 session
func (a *SessionAnalyzer) AnalyzeAll() ([]SessionInfo, error) {
	workspaceDir := a.store.basePath

	// 检查 workspace 目录是否存在
	if _, err := os.Stat(workspaceDir); os.IsNotExist(err) {
		return []SessionInfo{}, nil
	}

	// 遍历所有 agent 目录
	entries, err := os.ReadDir(workspaceDir)
	if err != nil {
		return nil, fmt.Errorf("读取 workspace 目录失败(%s): %w", workspaceDir, err)
	}

	var allInfos []SessionInfo
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		agentName := entry.Name()
		// 检查 agent 的 workspace/memory 目录是否存在
		agentMemoryDir := filepath.Join(workspaceDir, agentName, "workspace", "memory")
		if _, err := os.Stat(agentMemoryDir); os.IsNotExist(err) {
			// 如果没有 memory 目录，可能是空 agent，跳过
			continue
		}

		infos, err := a.AnalyzeAgent(agentName)
		if err != nil {
			logger.L().Warn().Err(err).Str("agent", agentName).Msg("分析 agent session 失败")
			continue
		}

		allInfos = append(allInfos, infos...)
	}

	return allInfos, nil
}

// AnalyzeAgent 分析指定 agent 的所有 session
func (a *SessionAnalyzer) AnalyzeAgent(agentName string) ([]SessionInfo, error) {
	metadatas, err := a.store.ListSessions()
	if err != nil {
		return nil, err
	}

	var infos []SessionInfo
	for _, metadata := range metadatas {
		info := SessionInfo{
			AgentName:    metadata.AgentName,
			ChannelName:  metadata.ChannelName,
			ChatID:       metadata.ChatID,
			StoragePath:  a.store.GetMemoryDir(agentName),
			CreatedAt:    metadata.CreatedAt,
			UpdatedAt:    metadata.UpdatedAt,
			MessageCount: metadata.MessageCount,
			TokenCount:   metadata.TokenCount,
		}
		infos = append(infos, info)
	}

	return infos, nil
}

// PrintSessionInfo 打印 session 信息到控制台
func PrintSessionInfo(infos []SessionInfo) {
	if len(infos) == 0 {
		fmt.Println("未找到任何 session")
		return
	}

	fmt.Println("============================================")
	fmt.Printf("共找到 %d 个 session\n", len(infos))
	fmt.Println("============================================")
	fmt.Println()

	for i, info := range infos {
		fmt.Printf("[%d] Session 信息:\n", i+1)
		fmt.Printf("  名称: agent_%s_%s_%s\n", info.AgentName, info.ChannelName, info.ChatID)
		fmt.Printf("  Agent: %s\n", info.AgentName)
		fmt.Printf("  通道: %s\n", info.ChannelName)
		fmt.Printf("  Chat ID: %s\n", info.ChatID)
		fmt.Printf("  存储路径: %s\n", info.StoragePath)
		fmt.Printf("  创建时间: %s\n", info.CreatedAt.Format("2006-01-02 15:04:05"))
		fmt.Printf("  更新时间: %s\n", info.UpdatedAt.Format("2006-01-02 15:04:05"))
		fmt.Printf("  消息数量: %d\n", info.MessageCount)
		fmt.Printf("  Token 数量: %d\n", info.TokenCount)
		fmt.Println()
	}
}

// GetSessionFilename 从 session key 解析文件名
func GetSessionFilename(agentName, sessionKey string) string {
	// sessionKey 格式: channel::chatID
	parts := strings.SplitN(sessionKey, "::", 2)
	if len(parts) != 2 {
		return ""
	}
	channelName, chatID := parts[0], parts[1]
	return fmt.Sprintf("agent_%s_%s_%s.jsonl", agentName, channelName, chatID)
}
