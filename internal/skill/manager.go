package skill

import (
	util "brambleclaw/internal"
	"brambleclaw/internal/config/structs"
	"brambleclaw/internal/interfaces"
	"brambleclaw/internal/logger"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type SkillManager struct {
	registry   *SkillRegistry
	cfg        *structs.SkillConfig
	workspaces []string
	watcher    *SkillWatcher
	mu         sync.RWMutex
}

func NewSkillManager(cfg *structs.SkillConfig) *SkillManager {
	return &SkillManager{
		registry: NewSkillRegistry(),
		cfg:      cfg,
	}
}

func (m *SkillManager) Initialize(ctx context.Context, cfg any) error {
	skillCfg, ok := cfg.(*structs.SkillConfig)
	if !ok {
		return fmt.Errorf("invalid config type for SkillManager")
	}
	m.cfg = skillCfg

	if !m.cfg.Enabled {
		logger.L().Debug().Msg("Skill system disabled")
		return nil
	}

	// Create watcher if hot reload enabled
	if m.cfg.HotReload {
		var err error
		m.watcher, err = NewSkillWatcher(m, m.cfg)
		if err != nil {
			logger.L().Warn().Err(err).Msg("Failed to create skill watcher, hot reload disabled")
			m.watcher = nil
		}
	}

	// Scan personal scope (~/.brambleclaw/skills/)
	personalDir := skillCfg.PersonalSkillDir
	if personalDir == "" {
		personalDir = filepath.Join(util.GetSystemPath(), "skills")
	}
	if _, err := os.Stat(personalDir); err == nil {
		if err := m.scanDirectory(ctx, personalDir, ScopePersonal); err != nil {
			logger.L().Warn().Err(err).Str("dir", personalDir).Msg("Failed to scan personal skill directory")
		}
	}

	// Scan agents scope (~/.agents/skills/) - npx skills add 安装目录
	agentsDir := skillCfg.AgentsSkillDir
	if agentsDir == "" {
		if homeDir, err := os.UserHomeDir(); err == nil {
			agentsDir = filepath.Join(homeDir, ".agents", "skills")
		}
	}
	if agentsDir != "" {
		if _, err := os.Stat(agentsDir); err == nil {
			if err := m.scanDirectory(ctx, agentsDir, ScopePersonal); err != nil {
				logger.L().Warn().Err(err).Str("dir", agentsDir).Msg("Failed to scan agents skill directory")
			}
		}
	}

	return nil
}

// AddWorkspace 将 agent workspace 下的 skill 注册到 manager 中
func (m *SkillManager) AddWorkspace(ctx context.Context, workspacePath string) error {
	if !m.cfg.Enabled {
		return nil
	}

	projectDir := m.cfg.ProjectSkillDir
	if projectDir == "" {
		projectDir = filepath.Join(workspacePath, "skills")
	}

	m.mu.Lock()
	m.workspaces = append(m.workspaces, workspacePath)
	m.mu.Unlock()

	// Add to watcher first if enabled
	if m.watcher != nil {
		if err := m.watcher.AddWorkspaceDir(ctx, workspacePath); err != nil {
			logger.L().Warn().Err(err).Str("workspace", workspacePath).Msg("Failed to watch workspace skill dir")
		}
	}

	if _, err := os.Stat(projectDir); err == nil {
		return m.scanDirectory(ctx, projectDir, ScopeProject)
	} else if !os.IsNotExist(err) {
		return err
	}

	return nil
}

// scanDirectory 扫描 Skill 目录， 并将合法的 skill 注册到 manager 中
func (m *SkillManager) scanDirectory(ctx context.Context, dir string, scope Scope) error {
	logger.L().Debug().Str("dir", dir).Int("scope", int(scope)).Msg("Scanning skill directory")

	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		skillDir := filepath.Join(dir, entry.Name())
		skillFile := filepath.Join(skillDir, "SKILL.md")
		if _, err := os.Stat(skillFile); os.IsNotExist(err) {
			continue
		}

		content, err := os.ReadFile(skillFile)
		if err != nil {
			logger.L().Warn().Err(err).Str("path", skillFile).Msg("Failed to read SKILL.md")
			continue
		}

		fm, _, err := ParseSKILLMarkdown(content)
		if err != nil {
			// Try fallback
			name, _ := ExtractMetadataFallback(string(content))
			if name == "" {
				continue
			}
			fm.Name = name
			fm.UserInvocable = true
		}

		if fm.Name == "" {
			fm.Name = entry.Name()
		}

		if err := ValidateFrontmatter(&fm, entry.Name()); err != nil {
			logger.L().Warn().Err(err).Str("path", skillFile).Msg("Invalid skill frontmatter, skipped")
			continue
		}

		info, err := os.Stat(skillFile)
		if err != nil {
			continue
		}

		entry := &SkillEntry{
			meta:    fm,
			scope:   scope,
			dirPath: skillDir,
			modTime: info.ModTime(),
			active:  false,
		}

		if err := m.registerIfHigherPriority(ctx, entry); err != nil {
			logger.L().Warn().Err(err).Str("name", fm.Name).Msg("Failed to register skill")
		} else {
			logger.L().Debug().Str("name", fm.Name).Int("scope", int(scope)).Msg("Skill registered")
		}
	}

	return nil
}

func (m *SkillManager) registerIfHigherPriority(ctx context.Context, entry *SkillEntry) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	existing, err := m.registry.Get(ctx, entry.meta.Name)
	if err == nil {
		if entry.scope > existing.scope {
			if err := m.registry.Unregister(ctx, entry.meta.Name); err != nil {
				return err
			}
			return m.registry.Register(ctx, entry.meta.Name, entry)
		}
		return nil
	}
	return m.registry.Register(ctx, entry.meta.Name, entry)
}

func (m *SkillManager) Activate(ctx context.Context, name string, source string) (*SkillEntry, error) {
	entry, err := m.registry.Get(ctx, name)
	if err != nil {
		return nil, err
	}

	if source == "model" && entry.meta.DisableModelInvoke {
		return nil, ErrInvocationBlocked
	}

	if !entry.active {
		skillFile := filepath.Join(entry.dirPath, "SKILL.md")
		content, err := os.ReadFile(skillFile)
		if err != nil {
			return nil, err
		}
		fm, body, err := ParseSKILLMarkdown(content)
		if err != nil {
			name, _ := ExtractMetadataFallback(string(content))
			if name != "" {
				body = string(content)
			}
		}
		entry.SetContent(body)
		entry.meta = fm
	}

	return entry, nil
}

func (m *SkillManager) Execute(ctx context.Context, name string, args SkillInvocationArgs) (string, error) {
	entry, err := m.Activate(ctx, name, args.Source)
	if err != nil {
		return "", err
	}

	content := entry.Content()

	if len(args.Named) > 0 || len(args.Positional) > 0 {
		content = ResolveVariables(content, args)
	}

	if strings.Contains(content, "!`") {
		var injErr error
		content, injErr = InjectDynamicContext(ctx, content, entry.dirPath, m.cfg)
		if injErr != nil {
			logger.L().Warn().Err(injErr).Str("name", name).Msg("Dynamic context injection failed")
		}
	}

	return content, nil
}

func (m *SkillManager) BuildContextBlock() string {
	var sb strings.Builder
	sb.WriteString("<skills>\n")

	skills := m.registry.List(context.Background())
	for _, s := range skills {
		meta := s.Meta()
		sb.WriteString("  <skill>\n")
		sb.WriteString(fmt.Sprintf("    <name>%s</name>\n", escapeXML(meta.Name)))
		sb.WriteString(fmt.Sprintf("    <description>%s</description>\n", escapeXML(meta.Description)))
		sb.WriteString(fmt.Sprintf("    <invocation>Use /%s or call activate_skill with name=\"%s\"</invocation>\n", meta.Name, meta.Name))
		if len(meta.Arguments) > 0 {
			sb.WriteString("    <arguments>\n")
			for _, arg := range meta.Arguments {
				attr := ""
				if arg.Required {
					attr = " required=\"true\""
				}
				sb.WriteString(fmt.Sprintf("      <arg name=%q%s>%s</arg>\n", arg.Name, attr, escapeXML(arg.Description)))
			}
			sb.WriteString("    </arguments>\n")
		}
		sb.WriteString("  </skill>\n")
	}

	sb.WriteString("</skills>")
	return sb.String()
}

func (m *SkillManager) Registry() *SkillRegistry {
	return m.registry
}

func escapeXML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}

func (m *SkillManager) StartAll(ctx context.Context) error {
	if m.watcher != nil {
		if err := m.watcher.Start(ctx); err != nil {
			logger.L().Warn().Err(err).Msg("Failed to start skill watcher")
		}
	}
	return nil
}

func (m *SkillManager) StopAll(ctx context.Context) error {
	if m.watcher != nil {
		if err := m.watcher.Stop(); err != nil {
			logger.L().Warn().Err(err).Msg("Failed to stop skill watcher")
		}
	}
	return nil
}

func (m *SkillManager) Add(ctx context.Context, id string, item *SkillEntry) error {
	return m.registry.Register(ctx, id, item)
}

func (m *SkillManager) Remove(ctx context.Context, id string) error {
	return m.registry.Unregister(ctx, id)
}

func (m *SkillManager) Get(ctx context.Context, id string) (*SkillEntry, error) {
	return m.registry.Get(ctx, id)
}

func (m *SkillManager) List(ctx context.Context) []*SkillEntry {
	return m.registry.List(ctx)
}

func (m *SkillManager) ListMeta(ctx context.Context) []*SkillMeta {
	entries := m.registry.List(ctx)
	metas := make([]*SkillMeta, 0, len(entries))
	for _, e := range entries {
		meta := e.Meta()
		metas = append(metas, &meta)
	}
	return metas
}

func (m *SkillManager) Status() interfaces.ManagerStatus {
	return interfaces.StatusRunning
}
