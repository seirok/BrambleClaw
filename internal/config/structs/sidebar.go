package structs

import "brambleclaw/internal/logger"

// SidebarConfig TUI 右侧统计边栏配置
type SidebarConfig struct {
	Enabled  bool             `json:"enabled" yaml:"enabled"`
	Width    int              `json:"width" yaml:"width"`
	Sections []SidebarSection `json:"sections" yaml:"sections"`
}

// SidebarSection 单个统计 section 配置
type SidebarSection struct {
	Name    string `json:"name" yaml:"name"`
	Enabled bool   `json:"enabled" yaml:"enabled"`
}

// DefaultSidebarConfig 返回默认边栏配置
func DefaultSidebarConfig() SidebarConfig {
	return SidebarConfig{
		Enabled: true,
		Width:   30,
		Sections: []SidebarSection{
			{Name: "token_usage", Enabled: true},
			{Name: "hook_stats", Enabled: true},
			{Name: "model_info", Enabled: true},
			{Name: "sandbox", Enabled: false},
			{Name: "session", Enabled: false},
			{Name: "mcp", Enabled: false},
		},
	}
}

// Validate validates SidebarConfig and fills defaults.
// Returns whether there was a critical error (never for SidebarConfig).
func (c *SidebarConfig) Validate() (hasError bool) {
	defaults := DefaultSidebarConfig()

	if c.Width <= 0 {
		c.Width = defaults.Width
	}
	if c.Width > 60 {
		c.Width = 60
		logger.L().Warn().Int("width", c.Width).Msg("Sidebar width too large, capped at 60")
	}

	if len(c.Sections) == 0 {
		c.Sections = defaults.Sections
	}

	// Validate section names
	validNames := map[string]bool{
		"token_usage": true,
		"hook_stats":  true,
		"model_info":  true,
		"sandbox":     true,
		"session":     true,
		"mcp":         true,
	}

	for i, section := range c.Sections {
		if !validNames[section.Name] {
			logger.L().Warn().Str("name", section.Name).Msg("Invalid sidebar section name, removing")
			c.Sections = append(c.Sections[:i], c.Sections[i+1:]...)
		}
	}

	return false
}
