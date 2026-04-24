package command

import (
	"brambleclaw/internal/interfaces"
	"context"
	"errors"
	"fmt"
	"sync"
)

var (
	ErrCommandNotFound = errors.New("command not found")
	ErrCommandExists   = errors.New("command already exists")
)

// 编译时检查：确保 CommandRegistry 实现了 Registry[Command]
var _ interfaces.Registry[*interfaces.Command] = (*CommandRegistry)(nil)

type CommandRegistry struct {
	commands map[string]*interfaces.Command
	mu       sync.RWMutex
}

// NewCommandRegistry 创建实例
func NewCommandRegistry() *CommandRegistry {
	return &CommandRegistry{
		commands: make(map[string]*interfaces.Command),
	}
}

// Register 注册指令
func (r *CommandRegistry) Register(ctx context.Context, name string, cmd *interfaces.Command) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.commands[name]; exists {
		return fmt.Errorf("%w: %s", ErrCommandExists, name)
	}

	r.commands[name] = cmd
	return nil
}

// Unregister 注销指令
func (r *CommandRegistry) Unregister(ctx context.Context, name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.commands[name]; !exists {
		return fmt.Errorf("%w: %s", ErrCommandNotFound, name)
	}

	delete(r.commands, name)
	return nil
}

// Get 获取指令
func (r *CommandRegistry) Get(ctx context.Context, name string) (*interfaces.Command, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	cmd, ok := r.commands[name]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrCommandNotFound, name)
	}
	return cmd, nil
}

// List 返回所有指令
func (r *CommandRegistry) List(ctx context.Context) []*interfaces.Command {
	r.mu.RLock()
	defer r.mu.RUnlock()

	list := make([]*interfaces.Command, 0, len(r.commands))
	for _, cmd := range r.commands {
		list = append(list, cmd)
	}
	return list
}
