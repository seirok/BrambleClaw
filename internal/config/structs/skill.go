package structs

type SkillConfig struct {
	Enabled          bool   `json:"enabled" mapstructure:"enabled"`
	PersonalSkillDir string `json:"personal_skill_dir" mapstructure:"personal_skill_dir"`
	AgentsSkillDir   string `json:"agents_skill_dir" mapstructure:"agents_skill_dir"`
	ProjectSkillDir  string `json:"project_skill_dir" mapstructure:"project_skill_dir"`
	HotReload        bool   `json:"hot_reload" mapstructure:"hot_reload"`
	DebounceMs       int    `json:"debounce_ms" mapstructure:"debounce_ms"`
	CommandTimeoutMs int    `json:"command_timeout_ms" mapstructure:"command_timeout_ms"`
	MaxCommandOutput int    `json:"max_command_output" mapstructure:"max_command_output"`
}

func DefaultSkillConfig() SkillConfig {
	return SkillConfig{
		Enabled:          true,
		PersonalSkillDir: "",
		AgentsSkillDir:   "",
		ProjectSkillDir:  "",
		HotReload:        true,
		DebounceMs:       500,
		CommandTimeoutMs: 5000,
		MaxCommandOutput: 1048576, // 1MB
	}
}

func (c *SkillConfig) Validate() (hasError bool) {
	defaults := DefaultSkillConfig()
	if c.DebounceMs <= 0 {
		c.DebounceMs = defaults.DebounceMs
	}
	if c.CommandTimeoutMs <= 0 {
		c.CommandTimeoutMs = defaults.CommandTimeoutMs
	}
	if c.MaxCommandOutput <= 0 {
		c.MaxCommandOutput = defaults.MaxCommandOutput
	}
	return false
}
