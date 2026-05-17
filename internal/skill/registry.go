package skill

import (
	"context"
	"fmt"
	"neoclaw/internal/interfaces"
	"neoclaw/internal/registry"
)

var _ interfaces.Registry[*SkillEntry] = (*SkillRegistry)(nil)

type SkillRegistry struct {
	*registry.GenericRegistry[*SkillEntry]
}

func NewSkillRegistry() *SkillRegistry {
	return &SkillRegistry{
		GenericRegistry: registry.NewGenericRegistry[*SkillEntry](
			func(name string) error { return ErrSkillExists },
			func(name string) error { return ErrSkillNotFound },
			nil,
		),
	}
}

func (r *SkillRegistry) GetMeta(ctx context.Context, name string) (*SkillMeta, error) {
	entry, err := r.Get(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrSkillNotFound, name)
	}
	meta := entry.Meta()
	return &meta, nil
}
