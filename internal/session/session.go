package session

import (
	util "brambleclaw/internal"
	"brambleclaw/internal/messages"
	"brambleclaw/internal/store"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const maxFirstUserMessageLen = 80

type Session struct {
	Key               string                 `json:"key"`
	Messages          []messages.BaseMessage `json:"messages"` // 会话消息
	CreatedAt         time.Time              `json:"created_at"`
	UpdatedAt         time.Time              `json:"updated_at"`
	Summarized        int                    `json:"summarized"`          // 指向最古老的有效信息
	Modified          bool                   `json:"modified"`            // 自上次保存后是否修改过
	LastSavedChecksum string                 `json:"last_saved_checksum"` // 上次保存时的校验和，用于检测变化

	// 存储相关字段（非持久化）
	store     *store.FileStorage[Session]
	metaStore *store.FileStorage[SessionMetadata]
	meta      *SessionMetadata
}

// NewSession 创建新的 Session 实例
func NewSession(key string, dataDir string) *Session {
	return &Session{
		Key:       key,
		Messages:  []messages.BaseMessage{},
		CreatedAt: time.Now(),
		store:     store.NewFileStorage[Session](filepath.Join(dataDir, "memory")),
		metaStore: store.NewFileStorage[SessionMetadata](filepath.Join(dataDir, "memory", "meta_data")),
	}
}

// Name 返回 session 名称（Service 接口）
func (s *Session) Name() string {
	return s.Key
}

// Start 从磁盘加载 session 和 metadata（Service 接口）
func (s *Session) Start(ctx context.Context) error {
	// 加载 session 数据
	sessionFile := util.GetSessionFile(util.SessionKeyToFile(s.Key))
	if session, err := s.store.Load(ctx, sessionFile); err == nil {
		s.Messages = session.Messages
		s.CreatedAt = session.CreatedAt
		s.UpdatedAt = session.UpdatedAt
		s.Summarized = session.Summarized
		s.Modified = false
		s.LastSavedChecksum = session.LastSavedChecksum
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("加载 session 失败(%s): %w", s.Key, err)
	}

	// 加载 metadata
	metaFile := util.GetSessionMetaFile(util.SessionKeyToFile(s.Key))
	if meta, err := s.metaStore.Load(ctx, metaFile); err == nil {
		s.meta = meta
	} else if os.IsNotExist(err) {
		// 如果不存在，创建新的 metadata
		agentName, channelName, chatID, err := util.ParseSessionKey(s.Key)
		if err != nil {
			return fmt.Errorf("解析 session key 失败(%s): %w", s.Key, err)
		}
		s.meta = &SessionMetadata{
			AgentName:   agentName,
			ChannelName: channelName,
			ChatID:      chatID,
			CreatedAt:   time.Now(),
		}
	} else {
		return fmt.Errorf("加载 session meta 失败(%s): %w", s.Key, err)
	}

	return nil
}

// Stop 将 session 和 metadata 保存到磁盘（Service 接口）
func (s *Session) Stop(ctx context.Context) error {
	// 保存 session 数据
	if err := s.store.Save(ctx, util.GetSessionFile(util.SessionKeyToFile(s.Key)), s); err != nil {
		return fmt.Errorf("保存 session 失败(%s): %w", s.Key, err)
	}

	// 更新并保存 metadata
	if s.meta != nil {
		s.meta.UpdatedAt = time.Now()
		s.meta.MessageCount = len(s.Messages)
		if err := s.metaStore.Save(ctx, util.GetSessionMetaFile(util.SessionKeyToFile(s.Key)), s.meta); err != nil {
			return fmt.Errorf("保存 session meta 失败(%s): %w", s.Key, err)
		}
	}

	s.Modified = false
	return nil
}

// setStores 注入存储引用（由 Manager 调用）
func (s *Session) setStores(store *store.FileStorage[Session], metaStore *store.FileStorage[SessionMetadata]) {
	s.store = store
	s.metaStore = metaStore
}

// setMeta 注入 metadata 引用（由 Manager 调用）
func (s *Session) setMeta(meta *SessionMetadata) {
	s.meta = meta
}

// GetMetadata 获取 session 的 metadata
func (s *Session) GetMetadata() *SessionMetadata {
	return s.meta
}

type SessionMetadata struct {
	AgentName         string    `json:"agent_name"`
	ChannelName       string    `json:"channel_name"`
	ChatID            string    `json:"chat_id"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
	MessageCount      int       `json:"message_count"`
	TokenCount        int       `json:"token_count"`
	SessionSummary    string    `json:"session_summary,omitempty"` // 会话摘要（多条，带时间戳）
	FirstUserMessage  string    `json:"first_user_message,omitempty"`
}

func (s *Session) LoadHistory() []messages.BaseMessage {
	if s.Summarized >= len(s.Messages) {
		return []messages.BaseMessage{}
	}
	return s.Messages[s.Summarized:]
}

func (s *Session) AddMessage(msg messages.BaseMessage) {
	s.Messages = append(s.Messages, msg)
	s.Modified = true
}
