package agent

import (
	"brambleclaw/logger"
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// SessionMetadata session 元数据
type SessionMetadata struct {
	AgentName    string    `json:"agent_name"`
	ChannelName  string    `json:"channel_name"`
	ChatID       string    `json:"chat_id"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	MessageCount int       `json:"message_count"`
	TokenCount   int       `json:"token_count"`
}

// SessionStore session 存储管理器
type SessionStore struct {
	basePath string
}

// NewSessionStore 创建 session 存储管理器
func NewSessionStore(basePath string) *SessionStore {
	return &SessionStore{
		basePath: basePath,
	}
}

// GetMemoryDir 获取 agent 的 memory 目录
// 新的结构: {basePath}/{agentName}/workspace/memory
func (s *SessionStore) GetMemoryDir(agentName string) string {
	return filepath.Join(s.basePath, agentName, "workspace", "memory")
}

// GetAgentWorkspaceDir 获取 agent 的 workspace 根目录
// 结构: {basePath}/{agentName}/
func (s *SessionStore) GetAgentWorkspaceDir(agentName string) string {
	return filepath.Join(s.basePath, agentName)
}

// GetAgentWorkspaceSubDir 获取 agent workspace 下的子目录
// 结构: {basePath}/{agentName}/workspace/{subDir}
func (s *SessionStore) GetAgentWorkspaceSubDir(agentName, subDir string) string {
	return filepath.Join(s.basePath, agentName, "workspace", subDir)
}

// BuildSessionFilename 构建 session 文件名
func (s *SessionStore) BuildSessionFilename(agentName, channelName, chatID string) string {
	return fmt.Sprintf("agent_%s_%s_%s.jsonl", agentName, channelName, chatID)
}

// BuildMetadataFilename 构建元数据文件名
func (s *SessionStore) BuildMetadataFilename(agentName, channelName, chatID string) string {
	return fmt.Sprintf("agent_%s_%s_%s.meta.json", agentName, channelName, chatID)
}

// SaveSession 保存 session 到文件
func (s *SessionStore) SaveSession(agentName, channelName, chatID string, messages []AgentMessage) error {
	// 确保目录存在
	memoryDir := s.GetMemoryDir(agentName)
	if err := os.MkdirAll(memoryDir, 0755); err != nil {
		return fmt.Errorf("创建 memory 目录失败(%s): %w", memoryDir, err)
	}

	// 构建文件路径
	filename := s.BuildSessionFilename(agentName, channelName, chatID)
	filepath := filepath.Join(memoryDir, filename)

	// 以追加模式打开文件（JSONL 格式）
	file, err := os.OpenFile(filepath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("打开 session 文件失败(%s): %w", filepath, err)
	}
	defer file.Close()

	// 写入消息（每行一个 JSON 对象）
	encoder := json.NewEncoder(file)
	for _, msg := range messages {
		if err := encoder.Encode(msg); err != nil {
			return fmt.Errorf("写入消息失败: %w", err)
		}
	}

	// 保存元数据
	metadata := &SessionMetadata{
		AgentName:    agentName,
		ChannelName:  channelName,
		ChatID:       chatID,
		UpdatedAt:    time.Now(),
		MessageCount: len(messages),
	}
	// 尝试加载已有的元数据以获取创建时间
	_, existingMetadata, _ := s.LoadSession(agentName, channelName, chatID)
	if existingMetadata != nil {
		metadata.CreatedAt = existingMetadata.CreatedAt
	} else {
		metadata.CreatedAt = time.Now()
	}

	if err := s.SaveMetadata(metadata); err != nil {
		logger.L().Warn().Err(err).Str("agent", agentName).Msg("保存元数据失败")
	}

	logger.L().Debug().
		Str("agent", agentName).
		Str("channel", channelName).
		Str("chat_id", chatID).
		Int("message_count", len(messages)).
		Msg("session 保存成功")

	return nil
}

// LoadSession 从文件加载 session
func (s *SessionStore) LoadSession(agentName, channelName, chatID string) ([]AgentMessage, *SessionMetadata, error) {
	// 构建文件路径
	memoryDir := s.GetMemoryDir(agentName)
	filename := s.BuildSessionFilename(agentName, channelName, chatID)
	filepath := filepath.Join(memoryDir, filename)

	// 检查文件是否存在
	if _, err := os.Stat(filepath); os.IsNotExist(err) {
		// 文件不存在，返回空会话
		metadata := &SessionMetadata{
			AgentName:   agentName,
			ChannelName: channelName,
			ChatID:      chatID,
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}
		return []AgentMessage{}, metadata, nil
	}

	// 打开文件
	file, err := os.Open(filepath)
	if err != nil {
		return nil, nil, fmt.Errorf("打开 session 文件失败(%s): %w", filepath, err)
	}
	defer file.Close()

	// 读取消息
	var messages []AgentMessage
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		var msg AgentMessage
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			logger.L().Warn().Err(err).Str("line", line).Msg("解析消息失败，跳过")
			continue
		}
		messages = append(messages, msg)
	}

	if err := scanner.Err(); err != nil {
		return nil, nil, fmt.Errorf("读取 session 文件失败: %w", err)
	}

	// 加载或创建元数据
	metadata := s.loadOrCreateMetadata(agentName, channelName, chatID, messages)

	logger.L().Debug().
		Str("agent", agentName).
		Str("channel", channelName).
		Str("chat_id", chatID).
		Int("message_count", len(messages)).
		Msg("session 加载成功")

	return messages, metadata, nil
}

// loadOrCreateMetadata 加载或创建元数据
func (s *SessionStore) loadOrCreateMetadata(agentName, channelName, chatID string, messages []AgentMessage) *SessionMetadata {
	memoryDir := s.GetMemoryDir(agentName)
	metaFilename := s.BuildMetadataFilename(agentName, channelName, chatID)
	metaPath := filepath.Join(memoryDir, metaFilename)

	// 尝试加载现有元数据
	if data, err := os.ReadFile(metaPath); err == nil {
		var metadata SessionMetadata
		if err := json.Unmarshal(data, &metadata); err == nil {
			// 更新消息计数
			metadata.MessageCount = len(messages)
			metadata.UpdatedAt = time.Now()
			return &metadata
		}
	}

	// 创建新元数据
	var createdAt time.Time
	if len(messages) > 0 {
		createdAt = time.UnixMilli(messages[0].Timestamp)
	} else {
		createdAt = time.Now()
	}

	metadata := &SessionMetadata{
		AgentName:    agentName,
		ChannelName:  channelName,
		ChatID:       chatID,
		CreatedAt:    createdAt,
		UpdatedAt:    time.Now(),
		MessageCount: len(messages),
		TokenCount:   0, // TODO: 计算 token 数
	}

	return metadata
}

// SaveMetadata 保存元数据
func (s *SessionStore) SaveMetadata(metadata *SessionMetadata) error {
	memoryDir := s.GetMemoryDir(metadata.AgentName)
	if err := os.MkdirAll(memoryDir, 0755); err != nil {
		return fmt.Errorf("创建 memory 目录失败(%s): %w", memoryDir, err)
	}

	metaFilename := s.BuildMetadataFilename(metadata.AgentName, metadata.ChannelName, metadata.ChatID)
	metaPath := filepath.Join(memoryDir, metaFilename)

	data, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化元数据失败: %w", err)
	}

	if err := os.WriteFile(metaPath, data, 0644); err != nil {
		return fmt.Errorf("保存元数据失败(%s): %w", metaPath, err)
	}

	return nil
}

// ListSessions 列出所有 session
func (s *SessionStore) ListSessions(agentName string) ([]SessionMetadata, error) {
	memoryDir := s.GetMemoryDir(agentName)

	// 检查目录是否存在
	if _, err := os.Stat(memoryDir); os.IsNotExist(err) {
		return []SessionMetadata{}, nil
	}

	entries, err := os.ReadDir(memoryDir)
	if err != nil {
		return nil, fmt.Errorf("读取 memory 目录失败(%s): %w", memoryDir, err)
	}

	var metadatas []SessionMetadata
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		// 只处理元数据文件
		if !strings.HasSuffix(name, ".meta.json") {
			continue
		}

		metaPath := filepath.Join(memoryDir, name)
		data, err := os.ReadFile(metaPath)
		if err != nil {
			logger.L().Warn().Err(err).Str("path", metaPath).Msg("读取元数据文件失败")
			continue
		}

		var metadata SessionMetadata
		if err := json.Unmarshal(data, &metadata); err != nil {
			logger.L().Warn().Err(err).Str("path", metaPath).Msg("解析元数据文件失败")
			continue
		}

		metadatas = append(metadatas, metadata)
	}

	return metadatas, nil
}

// DeleteSession 删除 session
func (s *SessionStore) DeleteSession(agentName, channelName, chatID string) error {
	memoryDir := s.GetMemoryDir(agentName)

	// 删除数据文件
	filename := s.BuildSessionFilename(agentName, channelName, chatID)
	sessionPath := filepath.Join(memoryDir, filename)
	if err := os.Remove(sessionPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("删除 session 文件失败(%s): %w", sessionPath, err)
	}

	// 删除元数据文件
	metaFilename := s.BuildMetadataFilename(agentName, channelName, chatID)
	metaPath := filepath.Join(memoryDir, metaFilename)
	if err := os.Remove(metaPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("删除元数据文件失败(%s): %w", metaPath, err)
	}

	logger.L().Debug().
		Str("agent", agentName).
		Str("channel", channelName).
		Str("chat_id", chatID).
		Msg("session 删除成功")

	return nil
}
