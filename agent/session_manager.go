package agent

import (
	util "brambleclaw/internal"
	"brambleclaw/logger"
	"brambleclaw/store"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	SessionSuffix = ".jsonl"
	MetaSuffix    = ".meta.json"
)

// PersistentSessionManager 支持持久化的 Session 管理器
type PersistentSessionManager struct {
	sessions         map[string]*Session // session key --> session
	sessionsMeta     map[string]*SessionMetadata
	workspace        string
	sessionStore     *store.FileStorage[Session]
	sessionMetaStore *store.FileStorage[SessionMetadata]
	mu               sync.RWMutex
	autosaveInterval time.Duration
	autosaveEnabled  bool
	stopChan         chan struct{}
	context          context.Context
}

func (psm *PersistentSessionManager) Start() {
	if psm.autosaveEnabled {
		go psm.autosaveLoop()
	}
}

// NewPersistentSessionManager 创建持久化 Session 管理器
func NewPersistentSessionManager(workspace string) *PersistentSessionManager {
	mgr := &PersistentSessionManager{
		sessions:         make(map[string]*Session),
		sessionsMeta:     make(map[string]*SessionMetadata),
		workspace:        workspace,
		sessionStore:     store.NewFileStorage[Session](filepath.Join(workspace, "memory")),
		sessionMetaStore: store.NewFileStorage[SessionMetadata](filepath.Join(workspace, "memory", "meta_data")),
		stopChan:         make(chan struct{}),
		autosaveEnabled:  true,
		autosaveInterval: time.Second * 5,
		context:          context.Background(),
		mu:               sync.RWMutex{},
	}
	return mgr
}

func (m *PersistentSessionManager) LoadAllSessionsWithMeta() error {
	if err := m.LoadSessions(); err != nil {
		return err
	}

	if err := m.LoadSessionsMeta(); err != nil {
		return err
	}

	return nil
}

// LoadSessions 加载所有 session
func (m *PersistentSessionManager) LoadSessions() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	// 1. 遍历存储目录下的所有文件
	// 我们假设 session 文件都直接放在 DataDir 下
	files, err := os.ReadDir(m.sessionStore.DataDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // 目录不存在说明还没数据，正常返回
		}
		return fmt.Errorf("failed to read dir: %w", err)
	}

	for _, file := range files {
		if file.IsDir() || !strings.HasSuffix(file.Name(), SessionSuffix) {
			continue
		}

		session, err := m.sessionStore.Load(m.context, file.Name())
		if err != nil {
			logger.L().Error().Err(err).Str("file", file.Name()).Msg("failed to load")
			continue
		}

		sessionKey := strings.TrimSuffix(file.Name(), SessionSuffix)

		m.sessions[sessionKey] = session
	}
	return nil
}

func (m *PersistentSessionManager) LoadSessionsMeta() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	files, err := os.ReadDir(m.sessionMetaStore.DataDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // 目录不存在说明还没数据，正常返回
		}
		return fmt.Errorf("failed to read dir: %w", err)
	}

	for _, file := range files {
		if file.IsDir() || !strings.HasSuffix(file.Name(), MetaSuffix) {
			continue
		}

		sessionMeta, err := m.sessionMetaStore.Load(m.context, file.Name())
		if err != nil {
			logger.L().Error().Err(err).Str("file", file.Name()).Msg("failed to load")
			continue
		}

		sessionKey := strings.TrimSuffix(file.Name(), MetaSuffix)

		m.sessionsMeta[sessionKey] = sessionMeta
	}

	return nil
}

// GetOrCreate 获取或创建 session
func (m *PersistentSessionManager) GetOrCreate(sessionkey string) (*Session, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if sess, ok := m.sessions[sessionkey]; ok {
		return sess, false
	}

	sess := &Session{
		Key:        sessionkey,
		Messages:   []AgentMessage{},
		CreatedAt:  time.Now(),
		Summarized: 0,
	}
	m.sessions[sessionkey] = sess

	if _, exists := m.sessionsMeta[sessionkey]; !exists {
		agentName, channelName, chatID, err := util.ParseSessionKey(sessionkey)
		if err != nil {
			logger.L().Error().Err(err).Str("sessionkey", sessionkey).Msg("failed to parse session key")
		}

		m.sessionsMeta[sessionkey] = &SessionMetadata{
			// 初始化你的 Meta 字段，例如：
			AgentName:    agentName,
			ChannelName:  channelName,
			ChatID:       chatID,
			CreatedAt:    time.Now(),
			MessageCount: 0,
			TokenCount:   0,
		}
	}

	return sess, true

}

// Get 获取 session
func (m *PersistentSessionManager) Get(key string) (*Session, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	sess, ok := m.sessions[key]
	return sess, ok
}

// ClearSession 清空指定 session 的消息
// 返回清空前消息数量和错误
func (m *PersistentSessionManager) ClearSession(sessionKey string) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	sess, ok := m.sessions[sessionKey]
	if !ok {
		return 0, fmt.Errorf("session not found: %s", sessionKey)
	}

	sessMeta, ok := m.sessionsMeta[sessionKey]
	if !ok {
		return 0, fmt.Errorf("session 和 sessionMeta 状态不一致: %s", sessionKey)
	}

	// 记录清空前消息数量
	count := len(sess.Messages)

	// 删除session
	if err := m.sessionStore.Delete(m.context, sessionKey+SessionSuffix); err != nil {
		return 0, err
	}
	if err := m.sessionMetaStore.Delete(m.context, sessionKey+MetaSuffix); err != nil {
		return 0, err
	}

	// 同步更新到内存
	sess.Messages = []AgentMessage{}
	sess.Modified = true
	sess.UpdatedAt = time.Now()
	sess.Summarized = 0

	sessMeta.TokenCount = 0
	sessMeta.MessageCount = 0
	sessMeta.UpdatedAt = time.Now()

	return count, nil
}

// Update 更新 session 和 session meta 的内存状态
func (m *PersistentSessionManager) Update(session *Session, tokenUsed int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	session.UpdatedAt = time.Now()
	session.Modified = true
	m.sessions[session.Key] = session

	sessionMeta, ok := m.sessionsMeta[session.Key]
	if !ok {
		logger.L().Error().Msg("session 和 sessionMeta 状态不一致")
		return
	}

	sessionMeta.UpdatedAt = time.Now()
	sessionMeta.MessageCount = len(session.Messages)
	sessionMeta.TokenCount = tokenUsed

}

func (m *PersistentSessionManager) SaveSessionMeta(sessionKey string) error {
	m.mu.RLock()
	sessMeta, ok := m.sessionsMeta[sessionKey]
	m.mu.RUnlock()

	if !ok {
		return fmt.Errorf("session meta不存在: %s", sessionKey)
	}

	if err := m.sessionMetaStore.Save(m.context, sessionKey+MetaSuffix, sessMeta); err != nil {
		return err
	}

	return nil
}

// SaveSession 保存指定 session
func (m *PersistentSessionManager) SaveSession(sessionKey string) error {
	m.mu.RLock()
	sess, ok := m.sessions[sessionKey]
	if !ok {
		m.mu.RUnlock()
		return fmt.Errorf("指定的session key: %s 不存在", sessionKey)
	}

	// 记录下准备保存的那一刻的时间
	saveStartedAt := sess.UpdatedAt
	m.mu.RUnlock()

	// IO 操作（不带锁）
	if err := m.sessionStore.Save(m.context, sessionKey+SessionSuffix, sess); err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	// 核心检查：如果在 IO 期间 UpdatedAt 没变，说明没有新修改，可以安全置为 false
	if sess.UpdatedAt.Equal(saveStartedAt) {
		sess.Modified = false
	}

	return nil
}

// SaveAllSessions 保存所有 session
func (m *PersistentSessionManager) SaveAllSessionsWithMeta() {
	m.mu.RLock()
	sessions := make([]*Session, 0, len(m.sessions))
	for _, sess := range m.sessions {
		if sess.Modified {
			sessions = append(sessions, sess)
		}
	}
	m.mu.RUnlock()

	for _, sess := range sessions {
		if err := m.SaveSession(sess.Key); err != nil {
			logger.L().Error().Err(err).Str("session_key", sess.Key).Msg("保存 session 失败")
		}
		if err := m.SaveSessionMeta(sess.Key); err != nil {
			logger.L().Error().Err(err).Str("session_key", sess.Key).Msg("保存 session meta 失败")
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
			m.SaveAllSessionsWithMeta()
		case <-m.stopChan:
			return
		}
	}
}

// Stop 停止管理器
func (m *PersistentSessionManager) Stop() {
	close(m.stopChan)

	// 最后一次保存
	m.SaveAllSessionsWithMeta()
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
