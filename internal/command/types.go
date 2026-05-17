package command

import (
	"context"
	"errors"
	"fmt"
	"neoclaw/internal/interfaces"
	"sync"
)

var (
	ErrCommandNotFound = errors.New("command not found")
	ErrCommandExists   = errors.New("command already exists")
)

// 编译时检查
var _ interfaces.Registry[interfaces.Command] = (*CommandRegistry)(nil)

type CommandRegistry struct {
	commands map[string]interfaces.Command
	mu       sync.RWMutex
}

func NewCommandRegistry() *CommandRegistry {
	return &CommandRegistry{
		commands: make(map[string]interfaces.Command),
	}
}

func (r *CommandRegistry) Register(ctx context.Context, name string, cmd interfaces.Command) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.commands[name]; exists {
		return fmt.Errorf("%w: %s", ErrCommandExists, name)
	}

	r.commands[name] = cmd
	return nil
}

func (r *CommandRegistry) Unregister(ctx context.Context, name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.commands[name]; !exists {
		return fmt.Errorf("%w: %s", ErrCommandNotFound, name)
	}

	delete(r.commands, name)
	return nil
}

func (r *CommandRegistry) Get(ctx context.Context, name string) (interfaces.Command, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	cmd, ok := r.commands[name]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrCommandNotFound, name)
	}
	return cmd, nil
}

func (r *CommandRegistry) List(ctx context.Context) []interfaces.Command {
	r.mu.RLock()
	defer r.mu.RUnlock()

	list := make([]interfaces.Command, 0, len(r.commands))
	for _, cmd := range r.commands {
		list = append(list, cmd)
	}
	return list
}
