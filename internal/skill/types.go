package skill

import (
	"errors"
	"time"
)

type Scope int

const (
	ScopePlugin     Scope = iota // 0: plugin-provided skills
	ScopeProject                 // 1: {workspace}/skills/
	ScopePersonal                // 2: ~/.brambleclaw/skills/
	ScopeEnterprise              // 3: enterprise-managed (future)
)

type SkillFrontmatter struct {
	Name               string         `yaml:"name"`
	Description        string         `yaml:"description"`
	DisableModelInvoke bool           `yaml:"disable-model-invocation"`
	UserInvocable      bool           `yaml:"user-invocable"`
	Arguments          []SkillArg     `yaml:"arguments"`
	AllowedTools       string         `yaml:"allowed-tools"`
	License            string         `yaml:"license,omitempty"`
	Compatibility      string         `yaml:"compatibility,omitempty"`
	Metadata           map[string]any `yaml:"metadata,omitempty"`
}

type SkillArg struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description,omitempty"`
	Required    bool   `yaml:"required,omitempty"`
	Default     string `yaml:"default,omitempty"`
}

type SkillEntry struct {
	meta    SkillFrontmatter
	scope   Scope
	dirPath string
	content string
	modTime time.Time
	active  bool
}

type SkillMeta struct {
	Name               string
	Description        string
	UserInvocable      bool
	DisableModelInvoke bool
	Scope              Scope
	DirPath            string
	Arguments          []SkillArg
}

type SkillInvocationArgs struct {
	Positional []string
	Named      map[string]string
	Source     string // "model", "user", "command"
	Env        map[string]string
}

var (
	ErrSkillNotFound      = errors.New("skill not found")
	ErrSkillExists        = errors.New("skill already exists")
	ErrInvalidFrontmatter = errors.New("invalid skill frontmatter")
	ErrNameMismatch       = errors.New("skill name does not match directory name")
	ErrInvocationBlocked  = errors.New("skill auto-invocation is disabled")
)

func (e *SkillEntry) Meta() SkillMeta {
	return SkillMeta{
		Name:               e.meta.Name,
		Description:        e.meta.Description,
		UserInvocable:      e.meta.UserInvocable,
		DisableModelInvoke: e.meta.DisableModelInvoke,
		Scope:              e.scope,
		DirPath:            e.dirPath,
		Arguments:          e.meta.Arguments,
	}
}

func (e *SkillEntry) Content() string {
	return e.content
}

func (e *SkillEntry) Active() bool {
	return e.active
}

func (e *SkillEntry) SetContent(content string) {
	e.content = content
	e.active = true
}

func (e *SkillEntry) SetModTime(t time.Time) {
	e.modTime = t
}
