package session

import (
	"context"
	"fmt"
	"sync"
)

// SessionRegistry 实现 interfaces.Registry[*Session]
type SessionRegistry struct {
	mu   sync.RWMutex
	data map[string]*Session
}

// NewSessionRegistry 创建新的 SessionRegistry
func NewSessionRegistry() *SessionRegistry {
	return &SessionRegistry{
		data: make(map[string]*Session),
	}
}

// Register 注册一个 session
func (r *SessionRegistry) Register(ctx context.Context, name string, value *Session) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.data[name]; exists {
		return fmt.Errorf("session 已存在: %s", name)
	}

	r.data[name] = value
	return nil
}

// Unregister 注销一个 session
func (r *SessionRegistry) Unregister(ctx context.Context, name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.data[name]; !exists {
		return fmt.Errorf("session 不存在: %s", name)
	}

	delete(r.data, name)
	return nil
}

// Get 获取一个 session
func (r *SessionRegistry) Get(ctx context.Context, name string) (*Session, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	sess, ok := r.data[name]
	if !ok {
		return nil, fmt.Errorf("session 不存在: %s", name)
	}
	return sess, nil
}

// List 列出所有 session
func (r *SessionRegistry) List(ctx context.Context) []*Session {
	r.mu.RLock()
	defer r.mu.RUnlock()

	sessions := make([]*Session, 0, len(r.data))
	for _, sess := range r.data {
		sessions = append(sessions, sess)
	}
	return sessions
}

// Has 检查 session 是否存在（扩展方法，非接口必需）
func (r *SessionRegistry) Has(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	_, exists := r.data[name]
	return exists
}

// Set 设置或覆盖 session（扩展方法，非接口必需）
func (r *SessionRegistry) Set(name string, value *Session) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.data[name] = value
}

// GetInternalMap 获取内部 map 引用（仅用于序列化/反序列化场景，谨慎使用）
func (r *SessionRegistry) GetInternalMap() map[string]*Session {
	return r.data
}
