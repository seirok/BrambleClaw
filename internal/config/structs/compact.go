package structs

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
