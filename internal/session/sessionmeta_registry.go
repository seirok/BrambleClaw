package session

import (
	"context"
	"fmt"
	"sync"
)

// SessionMetadataRegistry 实现 interfaces.Registry[*SessionMetadata]
type SessionMetadataRegistry struct {
	mu   sync.RWMutex
	data map[string]*SessionMetadata
}

// NewSessionMetadataRegistry 创建新的 SessionMetadataRegistry
func NewSessionMetadataRegistry() *SessionMetadataRegistry {
	return &SessionMetadataRegistry{
		data: make(map[string]*SessionMetadata),
	}
}

// Register 注册一个 session metadata
func (r *SessionMetadataRegistry) Register(ctx context.Context, name string, value *SessionMetadata) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.data[name]; exists {
		return fmt.Errorf("session metadata 已存在: %s", name)
	}

	r.data[name] = value
	return nil
}

// Unregister 注销一个 session metadata
func (r *SessionMetadataRegistry) Unregister(ctx context.Context, name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.data[name]; !exists {
		return fmt.Errorf("session metadata 不存在: %s", name)
	}

	delete(r.data, name)
	return nil
}

// Get 获取一个 session metadata
func (r *SessionMetadataRegistry) Get(ctx context.Context, name string) (*SessionMetadata, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	meta, ok := r.data[name]
	if !ok {
		return nil, fmt.Errorf("session metadata 不存在: %s", name)
	}
	return meta, nil
}

// List 列出所有 session metadata
func (r *SessionMetadataRegistry) List(ctx context.Context) []*SessionMetadata {
	r.mu.RLock()
	defer r.mu.RUnlock()

	metas := make([]*SessionMetadata, 0, len(r.data))
	for _, meta := range r.data {
		metas = append(metas, meta)
	}
	return metas
}

// Has 检查 session metadata 是否存在（扩展方法，非接口必需）
func (r *SessionMetadataRegistry) Has(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	_, exists := r.data[name]
	return exists
}

// Set 设置或覆盖 session metadata（扩展方法，非接口必需）
func (r *SessionMetadataRegistry) Set(name string, value *SessionMetadata) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.data[name] = value
}

// GetInternalMap 获取内部 map 引用（仅用于序列化/反序列化场景，谨慎使用）
func (r *SessionMetadataRegistry) GetInternalMap() map[string]*SessionMetadata {
	return r.data
}
