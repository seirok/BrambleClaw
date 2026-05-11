package session

import (
	util "brambleclaw/internal"
	"brambleclaw/internal/config"
	"brambleclaw/internal/interfaces"
	"brambleclaw/internal/logger"
	"brambleclaw/internal/messages"
	"brambleclaw/internal/store"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// PersistentSessionManager 支持持久化的 Session 管理器
type PersistentSessionManager struct {
	workspace        string
	currentSession   *Session
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
	m.autosaveInterval = time.Duration(sessionCfg.SaveInterval) * time.Second

	return nil
}

// StartAll 启动管理器，依次启动所有 Session
func (m *PersistentSessionManager) StartAll(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.ctx = ctx

	sessions := m.sessions.List(ctx)
	var errs []error
	for _, sess := range sessions {
		if err := sess.Start(ctx); err != nil {
			logger.L().Error().Err(err).Str("session_key", sess.Key).Msg("启动 session 失败")
			errs = append(errs, fmt.Errorf("session %q 启动失败: %w", sess.Key, err))
		}
	}

	if m.autosaveEnabled {
		go m.autosaveLoop()
	}

	m.status = interfaces.StatusRunning
	if len(errs) > 0 {
		return fmt.Errorf("sessions 启动出错: %v", errs)
	}
	return nil
}

// StopAll 停止管理器，依次停止所有 Session
func (m *PersistentSessionManager) StopAll(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 通知 autosave goroutine 退出
	select {
	case <-m.stopChan:
		// 已关闭，避免重复 close 导致 panic
	default:
		close(m.stopChan)
	}

	// 停止所有 Session（保存到磁盘）
	sessions := m.sessions.List(ctx)
	var errs []error
	for _, sess := range sessions {
		if err := sess.Stop(ctx); err != nil {
			errs = append(errs, fmt.Errorf("停止 session %q 失败: %w", sess.Key, err))
		}
	}

	m.status = interfaces.StatusStopped

	if len(errs) > 0 {
		return fmt.Errorf("停止 sessions 出错: %v", errs)
	}
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
	if err := m.sessionStore.Delete(m.ctx, util.GetSessionFile(util.SessionKeyToFile(id))); err != nil {
		logger.L().Error().Err(err).Str("session_key", id).Msg("删除 session 文件失败")
	}
	if err := m.sessionMetaStore.Delete(m.ctx, util.GetSessionMetaFile(util.SessionKeyToFile(id))); err != nil {
		logger.L().Error().Err(err).Str("session_key", id).Msg("删除 session meta 文件失败")
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

// GetCurrentSession 获取当前会话
func (m *PersistentSessionManager) GetCurrentSession() *Session {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.currentSession
}

// SetCurrentSession 设置当前会话
func (m *PersistentSessionManager) SetCurrentSession(session *Session) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.currentSession = session
}

// SaveCurrentSession 保存当前会话
func (m *PersistentSessionManager) SaveCurrentSession() error {
	m.mu.RLock()
	sess := m.currentSession
	m.mu.RUnlock()

	if sess == nil {
		return fmt.Errorf("no current session")
	}

	sess.GetMetadata().FirstUserMessage = sess.GetFirstUserMessage()

	if err := m.SaveSession(sess.Key); err != nil {
		return err
	}
	if err := m.SaveSessionMeta(sess.Key); err != nil {
		return err
	}

	return nil
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
	logger.L().Debug().Str("component", "Session").Msg("autosave loop start")
	ticker := time.NewTicker(m.autosaveInterval)
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

		sessMeta := sess.GetMetadata()
		if sessMeta.FirstUserMessage == "" {
			sessMeta.FirstUserMessage = sess.GetFirstUserMessage()
		}

		if err := m.SaveSessionMeta(sess.Key); err != nil {
			logger.L().Error().Err(err).Str("session_key", sess.Key).Msg("保存 session meta 失败")
		}
	}
}

// GetOrCreate 获取或创建 session
func (m *PersistentSessionManager) GetOrCreate(sessionkey string) (*Session, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 使用 Get 检查是否存在
	sess, err := m.sessions.Get(m.ctx, sessionkey)
	if err == nil {
		m.currentSession = sess
		return sess, false
	}

	sess = &Session{
		Key:        sessionkey,
		Messages:   []messages.BaseMessage{},
		CreatedAt:  time.Now(),
		Summarized: 0,
	}
	sess.setStores(m.sessionStore, m.sessionMetaStore)
	if err := m.sessions.Register(m.ctx, sessionkey, sess); err != nil {
		logger.L().Error().Err(err).Str("session_key", sessionkey).Msg("注册 session 失败")
	}

	// 检查 session meta 是否存在
	_, err = m.sessionsMeta.Get(m.ctx, sessionkey)
	if err != nil {
		agentName, channelName, chatID, err := util.ParseSessionKey(sessionkey)
		if err != nil {
			logger.L().Error().Err(err).Str("session_key", sessionkey).Msg("解析 session key 失败")
		}

		if err := m.sessionsMeta.Register(m.ctx, sessionkey, &SessionMetadata{
			AgentName:        agentName,
			ChannelName:      channelName,
			ChatID:           chatID,
			CreatedAt:        time.Now(),
			MessageCount:     0,
			TokenCount:       0,
			FirstUserMessage: "",
		}); err != nil {

			logger.L().Error().Err(err).Str("session_key", sessionkey).Msg("注册 session meta 失败")
		}
		// 将 meta 关联到 session
		if meta, err := m.sessionsMeta.Get(m.ctx, sessionkey); err == nil {
			sess.setMeta(meta)
		}
	}

	m.currentSession = sess
	return sess, true
}

// RenewSession 创建一个新的 session，保留旧 session 的持久化文件
func (m *PersistentSessionManager) RenewSession(sessionKey string) (newSessionKey string, oldMessageCount int, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 获取旧 session
	sess, err := m.sessions.Get(m.ctx, sessionKey)
	if err != nil {
		return "", 0, fmt.Errorf("session not found: %s", sessionKey)
	}

	oldMessageCount = len(sess.Messages)

	// 解析旧 session key 获取 agentName 和 channelName
	agentName, channelName, _, err := util.ParseSessionKey(sessionKey)
	if err != nil {
		return "", 0, fmt.Errorf("failed to parse session key: %w", err)
	}

	// 生成新的 chatID 和 session key
	newChatID := util.GenerateRandomChatID()
	newSessionKey = util.BuildSessionKey(agentName, channelName, newChatID)

	// 从注册中移除旧 session
	if err := m.sessions.Unregister(m.ctx, sessionKey); err != nil {
		// 继续，即使有错误
	}
	if err := m.sessionsMeta.Unregister(m.ctx, sessionKey); err != nil {
		// 继续，即使有错误
	}

	// 创建新 session
	newSess := &Session{
		Key:        newSessionKey,
		Messages:   []messages.BaseMessage{},
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
		Summarized: 0,
	}
	newSess.setStores(m.sessionStore, m.sessionMetaStore)

	if err := m.sessions.Register(m.ctx, newSessionKey, newSess); err != nil {
		return "", 0, fmt.Errorf("register new session failed: %w", err)
	}

	// 创建新的 session metadata
	newMeta := &SessionMetadata{
		AgentName:        agentName,
		ChannelName:      channelName,
		ChatID:           newChatID,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
		MessageCount:     0,
		TokenCount:       0,
		FirstUserMessage: "",
	}

	if err := m.sessionsMeta.Register(m.ctx, newSessionKey, newMeta); err != nil {
		return "", 0, fmt.Errorf("register new session meta failed: %w", err)
	}

	newSess.setMeta(newMeta)

	m.currentSession = newSess

	return newSessionKey, oldMessageCount, nil
}

// ClearSession 清空指定 session 的消息
func (m *PersistentSessionManager) ClearSessionWithMeta(sessionKey string) (int, error) {
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

	sess.Messages = []messages.BaseMessage{}
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
		logger.L().Error().Err(err).Str("session_key", session.Key).Msg("session 和 sessionMeta 状态不一致")
		return
	}

	// Auto-set first user message
	if sessionMeta.FirstUserMessage == "" {
		for _, msg := range session.Messages {
			if msg.GetSource() == "user" {
				sessionMeta.FirstUserMessage = truncateWithEllipsis(msg.ToText(), maxFirstUserMessageLen)
				break
			}
		}
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

	if err := m.sessionMetaStore.Save(m.ctx, util.GetSessionMetaFile(util.SessionKeyToFile(sessionKey)), sessMeta); err != nil {
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

	if err := m.sessionStore.Save(m.ctx, util.GetSessionFile(util.SessionKeyToFile(sessionKey)), sess); err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if sess.UpdatedAt.Equal(saveStartedAt) {
		sess.Modified = false
	}

	return nil
}

// LoadAllMetadata loads all session metadata from disk (even those not in registry)
func (m *PersistentSessionManager) LoadAllMetadata(ctx context.Context) ([]*SessionMetadata, error) {
	m.mu.RLock()
	dataDir := m.sessionMetaStore.DataDir
	m.mu.RUnlock()

	entries, err := os.ReadDir(dataDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []*SessionMetadata{}, nil
		}
		return nil, fmt.Errorf("failed to read metadata directory: %w", err)
	}

	var results []*SessionMetadata
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		fmt.Println(name)
		if !strings.HasSuffix(name, interfaces.MetaSuffix) {
			continue
		}

		m.mu.RLock()
		meta, err := m.sessionMetaStore.Load(ctx, name)
		m.mu.RUnlock()

		if err == nil {
			results = append(results, meta)
		}
	}
	logger.L().Debug().Int("sessions", len(results)).Msg("Session meta data has been loaded")
	return results, nil
}

// LoadSession loads a session from disk (creates if doesn't exist in registry)
func (m *PersistentSessionManager) LoadSession(ctx context.Context, sessionKey string) (*Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if already exists
	if sess, err := m.sessions.Get(ctx, sessionKey); err == nil {
		return sess, nil
	}

	// Create session object and load from disk
	sess := NewSession(sessionKey, m.workspace)
	sess.setStores(m.sessionStore, m.sessionMetaStore)

	if err := sess.Start(ctx); err != nil {
		return nil, fmt.Errorf("failed to load session: %w", err)
	}

	// Ensure metadata is in registry
	meta := sess.GetMetadata()
	if meta != nil {
		// Check if meta already exists first
		if _, err := m.sessionsMeta.Get(ctx, sessionKey); err != nil {
			if err := m.sessionsMeta.Register(ctx, sessionKey, meta); err != nil {
				logger.L().Error().Err(err).Str("session_key", sessionKey).Msg("register session meta failed")
			}
		}
		sess.setMeta(meta)
	}

	// Register session
	if err := m.sessions.Register(ctx, sessionKey, sess); err != nil {
		return nil, fmt.Errorf("failed to register session: %w", err)
	}

	m.currentSession = sess

	return sess, nil
}
