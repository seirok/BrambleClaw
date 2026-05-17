package command

import (
	"errors"
	"fmt"
	"neoclaw/internal/interfaces"
	"neoclaw/internal/registry"
)

var (
	ErrCommandNotFound = errors.New("command not found")
	ErrCommandExists   = errors.New("command already exists")
)

// 编译时检查
var _ interfaces.Registry[interfaces.Command] = (*CommandRegistry)(nil)

type CommandRegistry struct {
	*registry.GenericRegistry[interfaces.Command]
}

func NewCommandRegistry() *CommandRegistry {
	return &CommandRegistry{
		GenericRegistry: registry.NewGenericRegistry[interfaces.Command](
			func(name string) error { return fmt.Errorf("%w: %s", ErrCommandExists, name) },
			func(name string) error { return fmt.Errorf("%w: %s", ErrCommandNotFound, name) },
			nil,
		),
	}
}
