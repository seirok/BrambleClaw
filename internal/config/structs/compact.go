package structs

import "brambleclaw/internal/logger"

// CompactConfig 压缩配置
type CompactConfig struct {
	CompactThreshold    int  `json:"compact_threshold" mapstructure:"compact_threshold"`         // Token threshold to trigger compaction
	CompactRounds       int  `json:"compact_rounds" mapstructure:"compact_rounds"`               // Message interval to trigger compaction
	MaxSummaryLength    int  `json:"max_summary_length" mapstructure:"max_summary_length"`       // Max chars per summary (default 10000)
	EnableHierarchical  bool `json:"enable_hierarchical" mapstructure:"enable_hierarchical"`     // Enable summary-of-summaries
	HierarchicalDepth   int  `json:"hierarchical_depth" mapstructure:"hierarchical_depth"`       // Max depth (default 3)
	ArchiveOldSummaries bool `json:"archive_old_summaries" mapstructure:"archive_old_summaries"` // Archive vs delete old summaries
	PreserveKeyContext  bool `json:"preserve_key_context" mapstructure:"preserve_key_context"`   // Keep decisions/actions in compression
}

// DefaultCompactConfig 返回默认压缩配置
func DefaultCompactConfig() CompactConfig {
	return CompactConfig{
		CompactThreshold:    4000,
		CompactRounds:       20,
		MaxSummaryLength:    10000,
		EnableHierarchical:  false,
		HierarchicalDepth:   3,
		ArchiveOldSummaries: false,
		PreserveKeyContext:  true,
	}
}

// Validate validates CompactConfig and fills defaults.
// Returns whether there was a critical error (never for CompactConfig).
func (c *CompactConfig) Validate() (hasError bool) {
	defaults := DefaultCompactConfig()

	if c.CompactThreshold <= 0 {
		logger.L().Warn().Int("invalid_compact_threshold", c.CompactThreshold).Msg("Invalid compact_threshold, using default")
		c.CompactThreshold = defaults.CompactThreshold
	}

	if c.CompactRounds <= 0 {
		logger.L().Warn().Int("invalid_compact_rounds", c.CompactRounds).Msg("Invalid compact_rounds, using default")
		c.CompactRounds = defaults.CompactRounds
	}

	if c.MaxSummaryLength <= 0 {
		logger.L().Warn().Int("invalid_max_summary_length", c.MaxSummaryLength).Msg("Invalid max_summary_length, using default")
		c.MaxSummaryLength = defaults.MaxSummaryLength
	}

	if c.HierarchicalDepth < 1 || c.HierarchicalDepth > 5 {
		logger.L().Warn().Int("invalid_hierarchical_depth", c.HierarchicalDepth).Msg("Invalid hierarchical_depth, using default")
		c.HierarchicalDepth = defaults.HierarchicalDepth
	}

	return false
}
