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
	"path/filepath"
	"strings"
	"sync"
	"time"
)

func truncateWithEllipsis(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

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
			AgentName:    agentName,
			ChannelName:  channelName,
			ChatID:       chatID,
			CreatedAt:    time.Now(),
			MessageCount: 0,
			TokenCount:   0,
		}); err != nil {

			logger.L().Error().Err(err).Str("session_key", sessionkey).Msg("注册 session meta 失败")
		}
		// 将 meta 关联到 session
		if meta, err := m.sessionsMeta.Get(m.ctx, sessionkey); err == nil {
			sess.setMeta(meta)
		}
	}

	return sess, true
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

	if err := m.sessionStore.Delete(m.ctx, util.GetSessionFile(util.SessionKeyToFile(sessionKey))); err != nil {
		return 0, err
	}
	if err := m.sessionMetaStore.Delete(m.ctx, util.GetSessionMetaFile(util.SessionKeyToFile(sessionKey))); err != nil {
		return 0, err
	}

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
