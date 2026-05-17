package skill

import (
	"context"
	"neoclaw/internal/config/structs"
	"neoclaw/internal/logger"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

type SkillWatcher struct {
	watcher      *fsnotify.Watcher
	skillManager *SkillManager
	cfg          *structs.SkillConfig
	cancel       context.CancelFunc
	mu           sync.RWMutex
	debounce     map[string]time.Time
	watchingDirs map[string]bool
}

func NewSkillWatcher(sm *SkillManager, cfg *structs.SkillConfig) (*SkillWatcher, error) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	return &SkillWatcher{
		watcher:      w,
		skillManager: sm,
		cfg:          cfg,
		debounce:     make(map[string]time.Time),
		watchingDirs: make(map[string]bool),
	}, nil
}

func (sw *SkillWatcher) Start(ctx context.Context) error {
	if !sw.cfg.HotReload {
		logger.L().Debug().Msg("Hot reload disabled")
		return nil
	}

	ctx, cancel := context.WithCancel(ctx)
	sw.cancel = cancel

	// Add initial directories
	if err := sw.addInitialDirs(ctx); err != nil {
		cancel()
		return err
	}

	// Start watch loop
	go sw.watchLoop(ctx)

	// Start debounce cleanup
	go sw.debounceCleanupLoop(ctx)

	logger.L().Debug().Msg("Skill hot reload started")
	return nil
}

func (sw *SkillWatcher) Stop() error {
	if sw.cancel != nil {
		sw.cancel()
	}
	return sw.watcher.Close()
}

func (sw *SkillWatcher) addInitialDirs(ctx context.Context) error {
	sw.mu.Lock()
	defer sw.mu.Unlock()

	// Add personal skill dir (~/.neoclaw/skills/)
	personalDir := sw.cfg.PersonalSkillDir
	if personalDir == "" {
		personalDir = filepath.Join(getGlobalConfigPath(), "skills")
	}
	if _, err := os.Stat(personalDir); err == nil {
		if err := sw.addDir(ctx, personalDir); err != nil {
			logger.L().Warn().Err(err).Str("dir", personalDir).Msg("Failed to watch personal skill dir")
		}
	}

	// Add agents skill dir (~/.agents/skills/)
	agentsDir := sw.cfg.AgentsSkillDir
	if agentsDir == "" {
		if homeDir, err := os.UserHomeDir(); err == nil {
			agentsDir = filepath.Join(homeDir, ".agents", "skills")
		}
	}
	if agentsDir != "" {
		if _, err := os.Stat(agentsDir); err == nil {
			if err := sw.addDir(ctx, agentsDir); err != nil {
				logger.L().Warn().Err(err).Str("dir", agentsDir).Msg("Failed to watch agents skill dir")
			}
		}
	}

	// Add workspace dirs (from SkillManager)
	// We'll add them as they come in via AddWorkspace
	// For now, let's just watch what we have

	return nil
}

func (sw *SkillWatcher) AddWorkspaceDir(ctx context.Context, workspacePath string) error {
	sw.mu.Lock()
	defer sw.mu.Unlock()

	projectDir := sw.cfg.ProjectSkillDir
	if projectDir == "" {
		projectDir = filepath.Join(workspacePath, "skills")
	}

	if _, err := os.Stat(projectDir); err == nil {
		return sw.addDir(ctx, projectDir)
	} else if !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (sw *SkillWatcher) addDir(ctx context.Context, dir string) error {
	if sw.watchingDirs[dir] {
		return nil // Already watching
	}

	if err := sw.watcher.Add(dir); err != nil {
		return err
	}
	sw.watchingDirs[dir] = true
	logger.L().Debug().Str("dir", dir).Msg("Watching skill directory")
	return nil
}

func (sw *SkillWatcher) watchLoop(ctx context.Context) {
	for {
		select {
		case event, ok := <-sw.watcher.Events:
			if !ok {
				return
			}
			sw.handleEvent(ctx, event)
		case err, ok := <-sw.watcher.Errors:
			if !ok {
				return
			}
			logger.L().Error().Err(err).Msg("Skill watcher error")
		case <-ctx.Done():
			return
		}
	}
}

func (sw *SkillWatcher) handleEvent(ctx context.Context, event fsnotify.Event) {
	// We only care about SKILL.md files
	if filepath.Base(event.Name) != "SKILL.md" {
		return
	}

	// Debounce
	sw.mu.Lock()
	now := time.Now()
	sw.debounce[event.Name] = now
	sw.mu.Unlock()

	// Process after debounce delay
	delay := time.Duration(sw.cfg.DebounceMs) * time.Millisecond
	if delay <= 0 {
		delay = 500 * time.Millisecond
	}
	time.AfterFunc(delay, func() {
		sw.processEvent(ctx, event)
	})
}

func (sw *SkillWatcher) processEvent(ctx context.Context, event fsnotify.Event) {
	sw.mu.Lock()
	eventTime, ok := sw.debounce[event.Name]
	if !ok {
		sw.mu.Unlock()
		return
	}
	if time.Since(eventTime) < time.Duration(sw.cfg.DebounceMs)*time.Millisecond {
		sw.mu.Unlock()
		return // Debounced, another event came in
	}
	delete(sw.debounce, event.Name)
	sw.mu.Unlock()

	skillDir := filepath.Dir(event.Name)
	skillName := filepath.Base(skillDir)

	switch {
	case event.Has(fsnotify.Create) || event.Has(fsnotify.Write):
		// New or updated skill - re-scan the directory
		logger.L().Info().Str("skill", skillName).Msg("Skill updated, reloading")
		scope := sw.detectScope(skillDir)
		if err := sw.skillManager.scanDirectory(ctx, skillDir, scope); err != nil {
			logger.L().Error().Err(err).Str("skill", skillName).Msg("Failed to reload skill")
		}

	case event.Has(fsnotify.Remove):
		// Skill removed - unregister it
		logger.L().Info().Str("skill", skillName).Msg("Skill removed, unregistering")
		if err := sw.skillManager.Remove(ctx, skillName); err != nil {
			logger.L().Error().Err(err).Str("skill", skillName).Msg("Failed to unregister skill")
		}
	}
}

func (sw *SkillWatcher) detectScope(dir string) Scope {
	parentDir := filepath.Dir(dir)

	// Check if it's personal scope (~/.neoclaw/skills/)
	personalDir := sw.cfg.PersonalSkillDir
	if personalDir == "" {
		personalDir = filepath.Join(getGlobalConfigPath(), "skills")
	}
	if filepath.Clean(parentDir) == filepath.Clean(personalDir) {
		return ScopePersonal
	}

	// Check if it's agents scope (~/.agents/skills/)
	var agentsDir string
	if sw.cfg.AgentsSkillDir != "" {
		agentsDir = sw.cfg.AgentsSkillDir
	} else {
		// Fallback to ~/.agents/skills
		if homeDir, err := os.UserHomeDir(); err == nil {
			agentsDir = filepath.Join(homeDir, ".agents", "skills")
		}
	}
	if agentsDir != "" && filepath.Clean(parentDir) == filepath.Clean(agentsDir) {
		return ScopePersonal
	}

	// Default to project scope
	return ScopeProject
}

func (sw *SkillWatcher) debounceCleanupLoop(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			sw.cleanupDebounce()
		case <-ctx.Done():
			return
		}
	}
}

func (sw *SkillWatcher) cleanupDebounce() {
	sw.mu.Lock()
	defer sw.mu.Unlock()

	cutoff := time.Now().Add(-5 * time.Minute)
	for path, t := range sw.debounce {
		if t.Before(cutoff) {
			delete(sw.debounce, path)
		}
	}
}

// getGlobalConfigPath is a helper to get the global config path (copied from util to avoid import cycle)
func getGlobalConfigPath() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		// Fall back to current directory
		return "."
	}
	return filepath.Join(homeDir, ".neoclaw")
}
