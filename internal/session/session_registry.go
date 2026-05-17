package session

import (
	"fmt"
	"neoclaw/internal/registry"
)

// SessionRegistry 实现 interfaces.Registry[*Session]
type SessionRegistry struct {
	*registry.GenericRegistry[*Session]
}

// NewSessionRegistry 创建新的 SessionRegistry
func NewSessionRegistry() *SessionRegistry {
	return &SessionRegistry{
		GenericRegistry: registry.NewGenericRegistry[*Session](
			func(name string) error { return fmt.Errorf("session 已存在: %s", name) },
			func(name string) error { return fmt.Errorf("session 不存在: %s", name) },
			nil,
		),
	}
}
