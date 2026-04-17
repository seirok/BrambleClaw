package agent

import (
	"brambleclaw/logger"
	"fmt"
	"strings"
	"sync"
	"time"
)

// PersistentSessionManager 支持持久化的 Session 管理器
type PersistentSessionManager struct {
	sessions         map[string]*Session
	mu               sync.RWMutex
	store            *SessionStore
	agentName        string
	autosaveInterval time.Duration
	autosaveEnabled  bool
	stopChan         chan struct{}
}

func (psm *PersistentSessionManager) SetAutoSaveInterval(interval time.Duration) {
	psm.autosaveInterval = interval
	psm.autosaveEnabled = true
}

func (psm *PersistentSessionManager) Start() {
	if psm.autosaveEnabled && psm.autosaveInterval > 0 {
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
		autosaveEnabled: false,
	}
	return mgr
}

// LoadSessions 从存储加载所有 session
func (m *PersistentSessionManager) LoadSessions() error {
	metadata, err := m.store.ListSessions(m.agentName)
	if err != nil {
		return fmt.Errorf("列出 sessions 失败: %w", err)
	}

	for _, metadata := range metadata {
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
		sessionKey := metadata.ChannelName + "::" + metadata.ChatID
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

	return nil
}

// GetOrCreate 获取或创建 session
func (m *PersistentSessionManager) GetOrCreate(key string) *Session {
	m.mu.Lock()
	defer m.mu.Unlock()

	if sess, ok := m.sessions[key]; ok {
		return sess
	}

	sess := &Session{
		Key:       key,
		Messages:  []AgentMessage{},
		CreatedAt: time.Now(),
	}
	m.sessions[key] = sess
	return sess
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

// SaveSession 保存指定 session 到存储
func (m *PersistentSessionManager) SaveSession(sessionKey string) error {
	m.mu.RLock()
	sess, ok := m.sessions[sessionKey]
	m.mu.RUnlock()

	if !ok {
		return fmt.Errorf("session 不存在: %s", sessionKey)
	}

	// 解析 session key: channel::chatID
	parts := strings.SplitN(sessionKey, "::", 2)
	if len(parts) != 2 {
		return fmt.Errorf("无效的 session key: %s", sessionKey)
	}
	channelName, chatID := parts[0], parts[1]

	// 保存消息
	if err := m.store.SaveSession(m.agentName, channelName, chatID, sess.Messages); err != nil {
		return fmt.Errorf("保存 session 失败: %w", err)
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
func (m *PersistentSessionManager) SaveAllSessions() error {
	m.mu.RLock()
	sessions := make([]*Session, 0, len(m.sessions))
	for _, sess := range m.sessions {
		sessions = append(sessions, sess)
	}
	m.mu.RUnlock()

	var lastErr error
	for _, sess := range sessions {
		if err := m.SaveSession(sess.Key); err != nil {
			logger.L().Error().Err(err).Str("session_key", sess.Key).Msg("保存 session 失败")
			lastErr = err
		}
	}

	return lastErr
}

// autosaveLoop 自动保存循环
func (m *PersistentSessionManager) autosaveLoop() {
	logger.L().Debug().Msg("[Session] autosave loop start")
	ticker := time.NewTicker(m.autosaveInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := m.SaveAllSessions(); err != nil {
				logger.L().Error().Err(err).Msg("自动保存 sessions 失败")
			}
		case <-m.stopChan:
			return
		}
	}
}

// Stop 停止管理器
func (m *PersistentSessionManager) Stop() {
	close(m.stopChan)

	// 最后一次保存
	if err := m.SaveAllSessions(); err != nil {
		logger.L().Error().Err(err).Msg("停止时保存 sessions 失败")
	}
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
