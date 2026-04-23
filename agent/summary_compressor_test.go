package agent

import (
	"brambleclaw/config"
	"strings"
	"testing"
	"time"
)

func TestNewSummaryCompressor(t *testing.T) {
	cfg := config.CompactConfig{
		MaxSummaryLength:   10000,
		HierarchicalDepth:  3,
		PreserveKeyContext: true,
	}

	client := NewLLMClient(config.LLMConfig{})
	sc := NewSummaryCompressor(cfg, client)

	if sc == nil {
		t.Fatal("NewSummaryCompressor returned nil")
	}

	if sc.cfg.MaxSummaryLength != 10000 {
		t.Errorf("Expected MaxSummaryLength 10000, got %d", sc.cfg.MaxSummaryLength)
	}

	if len(sc.rootNodes) != 0 {
		t.Errorf("Expected empty rootNodes, got %d", len(sc.rootNodes))
	}
}

func TestAddSummary(t *testing.T) {
	cfg := config.CompactConfig{
		MaxSummaryLength:   100,
		HierarchicalDepth:  3,
		PreserveKeyContext: false,
	}

	sc := NewSummaryCompressor(cfg, nil)
	content := "Test summary content"
	timestamp := time.Now()

	node, err := sc.AddSummary(content, timestamp)
	if err != nil {
		t.Fatalf("AddSummary failed: %v", err)
	}

	if node == nil {
		t.Fatal("AddSummary returned nil node")
	}

	if node.Content != content {
		t.Errorf("Expected content '%s', got '%s'", content, node.Content)
	}

	if node.Level != 0 {
		t.Errorf("Expected level 0, got %d", node.Level)
	}

	if len(sc.rootNodes) != 1 {
		t.Errorf("Expected 1 root node, got %d", len(sc.rootNodes))
	}
}

func TestAddSummary_Truncation(t *testing.T) {
	cfg := config.CompactConfig{
		MaxSummaryLength:   10,
		HierarchicalDepth:  3,
		PreserveKeyContext: false,
	}

	sc := NewSummaryCompressor(cfg, nil)
	longContent := "This is a very long summary that should be truncated"

	node, err := sc.AddSummary(longContent, time.Now())
	if err != nil {
		t.Fatalf("AddSummary failed: %v", err)
	}

	if len(node.Content) > cfg.MaxSummaryLength {
		t.Errorf("Content should be truncated to %d chars, got %d", cfg.MaxSummaryLength, len(node.Content))
	}
}

func TestExtractKeyContext(t *testing.T) {
	cfg := config.CompactConfig{
		MaxSummaryLength:   10000,
		HierarchicalDepth:  3,
		PreserveKeyContext: true,
	}

	sc := NewSummaryCompressor(cfg, nil)

	content := `
User decided to implement a new feature.
Action: Created a new branch for development.
Result: All tests passed successfully.
We encountered an error when connecting to the database.
Finally, we completed the implementation.
`

	context := sc.ExtractKeyContext(content)

	expectedPatterns := []string{
		"implement a new feature",
		"Created a new branch for development",
		"All tests passed successfully",
		"when connecting to the database",
		"the implementation",
	}

	contextStr := strings.Join(context, " ")
	for _, pattern := range expectedPatterns {
		if !strings.Contains(contextStr, pattern) {
			t.Errorf("Expected context to contain '%s', got: %v", pattern, context)
		}
	}
}

func TestExtractKeyContext_Empty(t *testing.T) {
	cfg := config.CompactConfig{
		MaxSummaryLength:   10000,
		HierarchicalDepth:  3,
		PreserveKeyContext: true,
	}

	sc := NewSummaryCompressor(cfg, nil)

	content := "Just some regular conversation without key decisions."
	context := sc.ExtractKeyContext(content)

	// Should return empty or minimal results for content without patterns
	// The exact behavior depends on regex patterns
	if context == nil {
		t.Log("ExtractKeyContext returned nil for plain content (expected)")
	}
}

func TestGetHierarchy(t *testing.T) {
	cfg := config.CompactConfig{
		MaxSummaryLength:   10000,
		HierarchicalDepth:  3,
		PreserveKeyContext: false,
	}

	sc := NewSummaryCompressor(cfg, nil)

	// Add multiple summaries
	for i := 0; i < 3; i++ {
		_, err := sc.AddSummary("Summary "+string(rune('A'+i)), time.Now())
		if err != nil {
			t.Fatalf("AddSummary failed: %v", err)
		}
	}

	hierarchy := sc.GetHierarchy()

	if hierarchy == nil {
		t.Fatal("GetHierarchy returned nil")
	}

	if hierarchy.RootCount != 3 {
		t.Errorf("Expected 3 root nodes, got %d", hierarchy.RootCount)
	}

	if len(hierarchy.Nodes) != 3 {
		t.Errorf("Expected 3 nodes, got %d", len(hierarchy.Nodes))
	}
}

func TestArchive(t *testing.T) {
	cfg := config.CompactConfig{
		MaxSummaryLength:   10000,
		HierarchicalDepth:  3,
		PreserveKeyContext: false,
	}

	sc := NewSummaryCompressor(cfg, nil)

	// Add old summary
	oldTime := time.Now().Add(-48 * time.Hour)
	_, err := sc.AddSummary("Old summary", oldTime)
	if err != nil {
		t.Fatalf("AddSummary failed: %v", err)
	}

	// Add recent summary
	_, err = sc.AddSummary("Recent summary", time.Now())
	if err != nil {
		t.Fatalf("AddSummary failed: %v", err)
	}

	// Archive summaries older than 24 hours
	cutoff := time.Now().Add(-24 * time.Hour)
	archive, err := sc.Archive(cutoff)

	if err != nil {
		t.Fatalf("Archive failed: %v", err)
	}

	if archive == nil {
		t.Fatal("Archive returned nil")
	}

	if archive.NodeCount != 1 {
		t.Errorf("Expected 1 archived node, got %d", archive.NodeCount)
	}

	if len(sc.rootNodes) != 1 {
		t.Errorf("Expected 1 remaining root node, got %d", len(sc.rootNodes))
	}
}

func TestArchive_NoNodesToArchive(t *testing.T) {
	cfg := config.CompactConfig{
		MaxSummaryLength:   10000,
		HierarchicalDepth:  3,
		PreserveKeyContext: false,
	}

	sc := NewSummaryCompressor(cfg, nil)

	// Add only recent summary
	_, err := sc.AddSummary("Recent summary", time.Now())
	if err != nil {
		t.Fatalf("AddSummary failed: %v", err)
	}

	// Try to archive summaries older than 24 hours
	cutoff := time.Now().Add(-24 * time.Hour)
	_, err = sc.Archive(cutoff)

	if err == nil {
		t.Error("Expected error when no nodes to archive, got nil")
	}
}

func TestCompressNode(t *testing.T) {
	cfg := config.CompactConfig{
		MaxSummaryLength:   1000,
		HierarchicalDepth:  3,
		PreserveKeyContext: false,
	}

	// Create compressor without LLM client (will use truncation fallback)
	sc := NewSummaryCompressor(cfg, nil)

	// Add child nodes first
	child1, _ := sc.AddSummary("Child summary 1", time.Now())
	child2, _ := sc.AddSummary("Child summary 2", time.Now())

	// Link children to a parent (simulating tree structure)
	parent := &SummaryNode{
		ID:       generateNodeID(),
		Content:  "Parent summary",
		Level:    0,
		ChildIDs: []string{child1.ID, child2.ID},
	}
	sc.nodeIndex[parent.ID] = parent
	sc.rootNodes = append(sc.rootNodes, parent)

	// Compress the parent
	compressed, err := sc.CompressNode(parent)
	if err != nil {
		t.Fatalf("CompressNode failed: %v", err)
	}

	if compressed == nil {
		t.Fatal("CompressNode returned nil")
	}

	if !parent.Compressed {
		t.Error("Expected parent to be marked as compressed")
	}
}

func TestCompressNode_AlreadyCompressed(t *testing.T) {
	cfg := config.CompactConfig{
		MaxSummaryLength:   1000,
		HierarchicalDepth:  3,
		PreserveKeyContext: false,
	}

	sc := NewSummaryCompressor(cfg, nil)

	node, _ := sc.AddSummary("Test summary", time.Now())
	node.Compressed = true

	_, err := sc.CompressNode(node)
	if err == nil {
		t.Error("Expected error when compressing already compressed node")
	}
}

func TestGenerateNodeID(t *testing.T) {
	id1 := generateNodeID()
	id2 := generateNodeID()

	if id1 == "" {
		t.Error("generateNodeID returned empty string")
	}

	if id1 == id2 {
		t.Error("generateNodeID should generate unique IDs")
	}

	if len(id1) != 16 {
		t.Errorf("Expected ID length 16, got %d", len(id1))
	}
}
