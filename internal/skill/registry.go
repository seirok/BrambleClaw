package skill

import (
	"context"
	"fmt"
	"neoclaw/internal/interfaces"
	"sync"
)

var _ interfaces.Registry[*SkillEntry] = (*SkillRegistry)(nil)

type SkillRegistry struct {
	skills map[string]*SkillEntry
	mu     sync.RWMutex
}

func NewSkillRegistry() *SkillRegistry {
	return &SkillRegistry{
		skills: make(map[string]*SkillEntry),
	}
}

func (r *SkillRegistry) Register(ctx context.Context, name string, entry *SkillEntry) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.skills[name]; exists {
		return ErrSkillExists
	}

	r.skills[name] = entry
	return nil
}

func (r *SkillRegistry) Unregister(ctx context.Context, name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.skills[name]; !exists {
		return ErrSkillNotFound
	}

	delete(r.skills, name)
	return nil
}

func (r *SkillRegistry) Get(ctx context.Context, name string) (*SkillEntry, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	entry, ok := r.skills[name]
	if !ok {
		return nil, ErrSkillNotFound
	}
	return entry, nil
}

func (r *SkillRegistry) List(ctx context.Context) []*SkillEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()

	list := make([]*SkillEntry, 0, len(r.skills))
	for _, entry := range r.skills {
		list = append(list, entry)
	}
	return list
}

func (r *SkillRegistry) GetMeta(ctx context.Context, name string) (*SkillMeta, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	entry, ok := r.skills[name]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrSkillNotFound, name)
	}
	meta := entry.Meta()
	return &meta, nil
}

func (r *SkillRegistry) Update(ctx context.Context, name string, entry *SkillEntry) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.skills[name]; !exists {
		return ErrSkillNotFound
	}

	r.skills[name] = entry
	return nil
}
