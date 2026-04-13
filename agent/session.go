package agent

import (
	"log"
	"sync"
	"time"
)

// SessionManager 会话管理器
type SessionManager struct {
	sessions map[string]*Session
	mu       sync.RWMutex
}

// NewSessionManager 创建会话管理器
func NewSessionManager() *SessionManager {
	return &SessionManager{
		sessions: make(map[string]*Session),
		mu:       sync.RWMutex{},
	}
}

// GetOrCreate 获取或创建会话
func (m *SessionManager) GetOrCreate(key string) *Session {
	sess := &Session{
		Key:       key,
		CreatedAt: time.Now(),
	}
	m.sessions[sess.Key] = sess
	return sess
}

// Get 获取会话
func (m *SessionManager) Get(key string) (*Session, bool) {
	val, ok := m.sessions[key]
	if !ok {
		log.Printf("session %s not found", key)
		return nil, false
	}
	return val, true
}

// Update 更新会话
func (m *SessionManager) Update(session *Session) {
	m.sessions[session.Key] = session
	session.UpdatedAt = time.Now()
	log.Printf("session %s updated", session.Key)
	return
}

// GetHistory 获取会话历史
func (s *Session) GetHistory(max int) []AgentMessage {
	if len(s.Messages) < max {
		return s.Messages // 返回全部历史消息
	}
	if max < 0 {
		log.Printf("max is negative")
		return nil
	}
	return s.Messages[len(s.Messages)-max:]
}

// AddMessage 添加消息
func (s *Session) AddMessage(msg AgentMessage) {
	s.Messages = append(s.Messages, msg)
	s.UpdatedAt = time.Now()
}
