package session

import (
	"fmt"
	"neoclaw/internal/registry"
)

// SessionMetadataRegistry 实现 interfaces.Registry[*SessionMetadata]
type SessionMetadataRegistry struct {
	*registry.GenericRegistry[*SessionMetadata]
}

// NewSessionMetadataRegistry 创建新的 SessionMetadataRegistry
func NewSessionMetadataRegistry() *SessionMetadataRegistry {
	return &SessionMetadataRegistry{
		GenericRegistry: registry.NewGenericRegistry[*SessionMetadata](
			func(name string) error { return fmt.Errorf("session metadata 已存在: %s", name) },
			func(name string) error { return fmt.Errorf("session metadata 不存在: %s", name) },
			nil,
		),
	}
}

// Len 返回元数据数量
func (r *SessionMetadataRegistry) Len() int {
	count := 0
	r.Read(func(items map[string]*SessionMetadata) {
		count = len(items)
	})
	return count
}
