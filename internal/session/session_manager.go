package session

import (
	util "brambleclaw/internal"
	"brambleclaw/internal/config"
	"brambleclaw/internal/interfaces"
	"brambleclaw/internal/logger"
	"brambleclaw/internal/store"
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
	workspace        string
	sessions         interfaces.Registry[*Session] // session key --> session
	sessionsMeta     interfaces.Registry[*SessionMetadata]
	sessionStore     *store.FileStorage[Session]
	sessionMetaStore *store.FileStorage[SessionMetadata]
	mu               sync.RWMutex
	autosaveInterval time.Duration
	autosaveEnabled  bool
	stopChan         chan struct{}
	ctx              context.Context
	status           interfaces.ManagerStatus
}

// NewPersistentSessionManager 创建持久化 Session 管理器
func NewPersistentSessionManager(workspace string) *PersistentSessionManager {
	mgr := &PersistentSessionManager{
		sessions:         NewSessionRegistry(),
		sessionsMeta:     NewSessionMetadataRegistry(),
		workspace:        workspace, // i.e: $base_dir / agent name
		sessionStore:     store.NewFileStorage[Session](filepath.Join(workspace, "memory")),
		sessionMetaStore: store.NewFileStorage[SessionMetadata](filepath.Join(workspace, "memory", "meta_data")),
		stopChan:         make(chan struct{}),
		autosaveEnabled:  true,
		autosaveInterval: time.Second * 5,
		ctx:              context.Background(),
		status:           interfaces.StatusIdle,
		mu:               sync.RWMutex{},
	}
	return mgr
}

// Initialize 初始化 Session 管理器
func (m *PersistentSessionManager) Initialize(ctx context.Context, cfg any) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	sessionCfg, ok := cfg.(*config.SessionConfig)
	if !ok {
		return fmt.Errorf("invalid config type for AgentManager")
	}

	m.ctx = ctx
	m.status = interfaces.StatusRunning
	m.autosaveEnabled = sessionCfg.Enabled

	// 加载所有 session
	if err := m.LoadAllSessionsWithMeta(); err != nil {
		return fmt.Errorf("加载 sessions 失败: %w", err)
	}

	return nil
}

// Start 启动管理器
func (m *PersistentSessionManager) StartAll(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.ctx = ctx
	m.status = interfaces.StatusRunning

	if m.autosaveEnabled {
		go m.autosaveLoop()
	}

	return nil
}

// StopAll 停止管理器
func (m *PersistentSessionManager) StopAll(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	close(m.stopChan)
	m.status = interfaces.StatusStopped

	// 最后一次保存
	m.SaveAllSessionsWithMeta()

	return nil
}

// Add 添加一个 session
func (m *PersistentSessionManager) Add(ctx context.Context, id string, item *Session) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.sessions.Register(ctx, id, item)
}

// Remove 移除一个 session
func (m *PersistentSessionManager) Remove(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 删除存储文件
	if err := m.sessionStore.Delete(m.ctx, id+SessionSuffix); err != nil {
		logger.L().Error().Err(err).Str("session", id).Msg("删除 session 文件失败")
	}
	if err := m.sessionMetaStore.Delete(m.ctx, id+MetaSuffix); err != nil {
		logger.L().Error().Err(err).Str("session", id).Msg("删除 session meta 文件失败")
	}

	if err := m.sessions.Unregister(ctx, id); err != nil {
		return err
	}
	if err := m.sessionsMeta.Unregister(ctx, id); err != nil {
		return err
	}

	return nil
}

// Get 获取一个 session
func (m *PersistentSessionManager) Get(ctx context.Context, id string) (*Session, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.sessions.Get(ctx, id)
}

// List 返回所有 session 列表
func (m *PersistentSessionManager) List(ctx context.Context) []*Session {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.sessions.List(ctx)
}

// Status 返回管理器状态
func (m *PersistentSessionManager) Status() interfaces.ManagerStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.status
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

// LoadAllSessionsWithMeta 加载所有 session 和 metadata
func (m *PersistentSessionManager) LoadAllSessionsWithMeta() error {
	if err := m.LoadSessions(); err != nil {
		return err
	}

	if err := m.LoadSessionsMeta(); err != nil {
		return err
	}

	return nil
}

// SaveAllSessionsWithMeta 保存所有修改过的 session
func (m *PersistentSessionManager) SaveAllSessionsWithMeta() {
	m.mu.RLock()
	sessionList := m.sessions.List(m.ctx)
	sessions := make([]*Session, 0, len(sessionList))
	for _, sess := range sessionList {
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

// LoadSessions 加载所有 session
func (m *PersistentSessionManager) LoadSessions() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	files, err := os.ReadDir(m.sessionStore.DataDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("读取目录失败: %w", err)
	}

	for _, file := range files {
		if file.IsDir() || !strings.HasSuffix(file.Name(), SessionSuffix) {
			continue
		}

		session, err := m.sessionStore.Load(m.ctx, file.Name())
		if err != nil {
			logger.L().Error().Err(err).Str("file", file.Name()).Msg("加载 session 失败")
			continue
		}

		sessionKey := strings.TrimSuffix(file.Name(), SessionSuffix)
		// 加载时直接覆盖，先注销再注册
		_ = m.sessions.Unregister(m.ctx, sessionKey)
		if err := m.sessions.Register(m.ctx, sessionKey, session); err != nil {
			logger.L().Error().Err(err).Str("session_key", sessionKey).Msg("注册 session 失败")
		}
	}
	return nil
}

// LoadSessionsMeta 加载所有 session metadata
func (m *PersistentSessionManager) LoadSessionsMeta() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	files, err := os.ReadDir(m.sessionMetaStore.DataDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("读取目录失败: %w", err)
	}

	for _, file := range files {
		if file.IsDir() || !strings.HasSuffix(file.Name(), MetaSuffix) {
			continue
		}

		sessionMeta, err := m.sessionMetaStore.Load(m.ctx, file.Name())
		if err != nil {
			logger.L().Error().Err(err).Str("file", file.Name()).Msg("加载 session meta 失败")
			continue
		}

		sessionKey := strings.TrimSuffix(file.Name(), MetaSuffix)
		// 加载时直接覆盖，先注销再注册
		_ = m.sessionsMeta.Unregister(m.ctx, sessionKey)
		if err := m.sessionsMeta.Register(m.ctx, sessionKey, sessionMeta); err != nil {
			logger.L().Error().Err(err).Str("session_key", sessionKey).Msg("注册 session meta 失败")
		}
	}

	return nil
}

// GetOrCreate 获取或创建 session
func (m *PersistentSessionManager) GetOrCreate(sessionkey string) (*Session, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 使用 Get 检查是否存在
	sess, err := m.sessions.Get(m.ctx, sessionkey)
	if err == nil {
		return sess, false
	}

	sess = &Session{
		Key:        sessionkey,
		Messages:   []interfaces.Message{},
		CreatedAt:  time.Now(),
		Summarized: 0,
	}
	if err := m.sessions.Register(m.ctx, sessionkey, sess); err != nil {
		logger.L().Error().Err(err).Str("sessionkey", sessionkey).Msg("注册 session 失败")
	}

	// 检查 session meta 是否存在
	_, err = m.sessionsMeta.Get(m.ctx, sessionkey)
	if err != nil {
		agentName, channelName, chatID, err := util.ParseSessionKey(sessionkey)
		if err != nil {
			logger.L().Error().Err(err).Str("sessionkey", sessionkey).Msg("解析 session key 失败")
		}

		if err := m.sessionsMeta.Register(m.ctx, sessionkey, &SessionMetadata{
			AgentName:    agentName,
			ChannelName:  channelName,
			ChatID:       chatID,
			CreatedAt:    time.Now(),
			MessageCount: 0,
			TokenCount:   0,
		}); err != nil {
			logger.L().Error().Err(err).Str("sessionkey", sessionkey).Msg("注册 session meta 失败")
		}
	}

	return sess, true
}

// ClearSession 清空指定 session 的消息
func (m *PersistentSessionManager) ClearSession(sessionKey string) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	sess, err := m.sessions.Get(m.ctx, sessionKey)
	if err != nil {
		return 0, fmt.Errorf("session not found: %s", sessionKey)
	}

	sessMeta, err := m.sessionsMeta.Get(m.ctx, sessionKey)
	if err != nil {
		return 0, fmt.Errorf("session 和 sessionMeta 状态不一致: %s", sessionKey)
	}

	count := len(sess.Messages)

	if err := m.sessionStore.Delete(m.ctx, sessionKey+SessionSuffix); err != nil {
		return 0, err
	}
	if err := m.sessionMetaStore.Delete(m.ctx, sessionKey+MetaSuffix); err != nil {
		return 0, err
	}

	sess.Messages = []interfaces.Message{}
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
	// 注意：session 已经存在于 registry 中（通过指针引用），
	// 直接修改对象即可，无需重新注册

	sessionMeta, err := m.sessionsMeta.Get(m.ctx, session.Key)
	if err != nil {
		logger.L().Error().Err(err).Msg("session 和 sessionMeta 状态不一致")
		return
	}

	sessionMeta.UpdatedAt = time.Now()
	sessionMeta.MessageCount = len(session.Messages)
	sessionMeta.TokenCount = tokenUsed
}

// UpdateSessionSummary 更新 session 的摘要
func (m *PersistentSessionManager) UpdateSessionSummary(sessionKey string, summaryContent string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	meta, err := m.sessionsMeta.Get(m.ctx, sessionKey)
	if err != nil {
		return fmt.Errorf("session meta not found: %s", sessionKey)
	}

	timestamp := time.Now().Format("2006-01-02 15:04")
	newEntry := fmt.Sprintf("[%s] %s", timestamp, summaryContent)

	if meta.SessionSummary != "" {
		meta.SessionSummary = meta.SessionSummary + "\n---\n" + newEntry
	} else {
		meta.SessionSummary = newEntry
	}

	const maxSummaryLength = 10000
	if len(meta.SessionSummary) > maxSummaryLength {
		meta.SessionSummary = meta.SessionSummary[len(meta.SessionSummary)-maxSummaryLength:]
		if idx := strings.Index(meta.SessionSummary, "---\n["); idx > 0 {
			meta.SessionSummary = meta.SessionSummary[idx+4:]
		}
	}

	meta.UpdatedAt = time.Now()

	if err := m.SaveSessionMeta(sessionKey); err != nil {
		return err
	}

	return nil
}

// SaveSessionMeta 保存 session metadata
func (m *PersistentSessionManager) SaveSessionMeta(sessionKey string) error {
	m.mu.RLock()
	sessMeta, err := m.sessionsMeta.Get(m.ctx, sessionKey)
	m.mu.RUnlock()

	if err != nil {
		return fmt.Errorf("session meta 不存在: %s", sessionKey)
	}

	if err := m.sessionMetaStore.Save(m.ctx, sessionKey+MetaSuffix, sessMeta); err != nil {
		return err
	}

	return nil
}

// SaveSession 保存指定 session
func (m *PersistentSessionManager) SaveSession(sessionKey string) error {
	m.mu.RLock()
	sess, err := m.sessions.Get(m.ctx, sessionKey)
	if err != nil {
		m.mu.RUnlock()
		return fmt.Errorf("指定的 session key: %s 不存在", sessionKey)
	}

	saveStartedAt := sess.UpdatedAt
	m.mu.RUnlock()

	if err := m.sessionStore.Save(m.ctx, sessionKey+SessionSuffix, sess); err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if sess.UpdatedAt.Equal(saveStartedAt) {
		sess.Modified = false
	}

	return nil
}

// GetAllSessions 获取所有 session
func (m *PersistentSessionManager) GetAllSessions() []*Session {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.sessions.List(m.ctx)
}
