package tools

import (
	"brambleclaw/internal/interfaces"
	"context"
	"errors"
	"fmt"
	"sync"
)

// ToolRegistry 工具注册中心
var (
	ErrToolNotFound = errors.New("tool not found")
	ErrToolExists   = errors.New("tool already exists")
)

// 这一行强制要求编译器检查 ToolRegistry 是否实现了 Registry[Tool]
var _ interfaces.Registry[interfaces.Tool] = (*ToolRegistry)(nil)

type ToolRegistry struct {
	tools map[string]interfaces.Tool
	mu    sync.RWMutex
}

func NewToolRegistry() *ToolRegistry {
	return &ToolRegistry{
		tools: make(map[string]interfaces.Tool),
		mu:    sync.RWMutex{},
	}
}

func (r *ToolRegistry) Register(ctx context.Context, name string, value interfaces.Tool) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.tools[name]; exists {
		return fmt.Errorf("%w: %s", ErrToolExists, name)
	}

	r.tools[name] = value
	return nil
}

func (r *ToolRegistry) Unregister(ctx context.Context, name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.tools[name]; !exists {
		return fmt.Errorf("%w: %s", ErrToolNotFound, name)
	}

	delete(r.tools, name)
	return nil
}

func (r *ToolRegistry) Get(ctx context.Context, name string) (interfaces.Tool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	t, ok := r.tools[name]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrToolNotFound, name)
	}
	return t, nil
}

func (r *ToolRegistry) List(ctx context.Context) []interfaces.Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	list := make([]interfaces.Tool, 0, len(r.tools))
	for _, t := range r.tools {
		list = append(list, t)
	}
	return list
}
