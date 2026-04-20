package agent

import (
	util "brambleclaw/internal"
	"brambleclaw/logger"
	"fmt"
	"strings"
	"sync"
	"time"
)

// PersistentSessionManager 支持持久化的 Session 管理器
type PersistentSessionManager struct {
	sessions         map[string]*Session // session key --> session
	mu               sync.RWMutex
	store            *SessionStore
	agentName        string
	autosaveInterval time.Duration
	autosaveEnabled  bool
	stopChan         chan struct{}
}

func (psm *PersistentSessionManager) Start() {
	if psm.autosaveEnabled {
		go psm.autosaveLoop()
	}
}

// NewPersistentSessionManager 创建持久化 Session 管理器
func NewPersistentSessionManager(agentName, workspacePath string) *PersistentSessionManager {
	store := NewSessionStore(workspacePath)
	mgr := &PersistentSessionManager{
		sessions:        make(map[string]*Session),
		store:           store,
		agentName:       agentName,
		stopChan:        make(chan struct{}),
		autosaveEnabled: true,
	}
	return mgr
}

// LoadSessions 加载所有 session
func (m *PersistentSessionManager) LoadSessions() {
	metaData, err := m.store.ListSessions()
	if len(metaData) == 0 {
		logger.L().Debug().Msg("No sessions found")
	}

	if err != nil {
		logger.L().Error().Err(err).Msg("Error loading sessions")
	}

	for _, metadata := range metaData {
		messages, _, err := m.store.LoadSession(m.agentName, metadata.ChannelName, metadata.ChatID)
		if err != nil {
			logger.L().Warn().
				Err(err).
				Str("agent", m.agentName).
				Str("channel", metadata.ChannelName).
				Str("chat_id", metadata.ChatID).
				Msg("加载 session 失败，跳过")
			continue
		}

		// 构建 session key
		sessionKey := util.BuildSessionKey(m.agentName, metadata.ChannelName, metadata.ChatID)

		sess := &Session{
			Key:       sessionKey,
			Messages:  messages,
			CreatedAt: metadata.CreatedAt,
			UpdatedAt: metadata.UpdatedAt,
		}

		m.mu.Lock()
		m.sessions[sessionKey] = sess
		m.mu.Unlock()

		logger.L().Debug().
			Str("agent", m.agentName).
			Str("session_key", sessionKey).
			Int("message_count", len(messages)).
			Msg("session 加载成功")
	}
}

// GetOrCreate 获取或创建 session
func (m *PersistentSessionManager) GetOrCreate(key string) (*Session, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if sess, ok := m.sessions[key]; ok {
		return sess, false
	}

	sess := &Session{
		Key:        key,
		Messages:   []AgentMessage{},
		CreatedAt:  time.Now(),
		Summarized: 0,
	}
	m.sessions[key] = sess
	return sess, true
}

// Get 获取 session
func (m *PersistentSessionManager) Get(key string) (*Session, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	sess, ok := m.sessions[key]
	return sess, ok
}

// Update 更新 session
func (m *PersistentSessionManager) Update(session *Session) {
	m.mu.Lock()
	defer m.mu.Unlock()

	session.UpdatedAt = time.Now()
	m.sessions[session.Key] = session
}

// SaveSession 保存指定 session 和 对应的 meta data
func (m *PersistentSessionManager) SaveSession(sessionKey string) error {
	m.mu.RLock()
	sess, ok := m.sessions[sessionKey]
	m.mu.RUnlock()

	if !ok {
		return fmt.Errorf("session 不存在: %s", sessionKey)
	}

	// 解析 session key: channel::agent::chatID
	parts := strings.SplitN(sessionKey, "::", 3)
	if len(parts) != 3 {
		return fmt.Errorf("无效的 session key: %s", sessionKey)
	}
	channelName, chatID := parts[0], parts[2]

	// 保存消息
	if err := m.store.SaveSession(m.agentName, channelName, chatID, sess.Messages); err != nil {
		return err
	}

	// 更新并保存元数据
	metadata := &SessionMetadata{
		AgentName:    m.agentName,
		ChannelName:  channelName,
		ChatID:       chatID,
		CreatedAt:    sess.CreatedAt,
		UpdatedAt:    time.Now(),
		MessageCount: len(sess.Messages),
	}

	if err := m.store.SaveMetadata(metadata); err != nil {
		logger.L().Warn().Err(err).Str("session_key", sessionKey).Msg("保存元数据失败")
	}

	return nil
}

// SaveAllSessions 保存所有 session
func (m *PersistentSessionManager) SaveAllSessions() {
	m.mu.RLock()
	sessions := make([]*Session, 0, len(m.sessions))
	for _, sess := range m.sessions {
		sessions = append(sessions, sess)
	}
	m.mu.RUnlock()

	for _, sess := range sessions {
		if err := m.SaveSession(sess.Key); err != nil {
			logger.L().Error().Err(err).Str("session_key", sess.Key).Msg("保存 session 失败")
		}
	}

}

// autosaveLoop 自动保存循环
func (m *PersistentSessionManager) autosaveLoop() {
	logger.L().Debug().Msg("[Session] autosave loop start")
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			m.SaveAllSessions()
		case <-m.stopChan:
			return
		}
	}
}

// Stop 停止管理器
func (m *PersistentSessionManager) Stop() {
	close(m.stopChan)

	// 最后一次保存
	m.SaveAllSessions()
}

// GetAllSessions 获取所有 session（用于分析器）
func (m *PersistentSessionManager) GetAllSessions() []*Session {
	m.mu.RLock()
	defer m.mu.RUnlock()

	sessions := make([]*Session, 0, len(m.sessions))
	for _, sess := range m.sessions {
		sessions = append(sessions, sess)
	}
	return sessions
}
