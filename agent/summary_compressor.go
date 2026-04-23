package agent

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
	"time"

	"brambleclaw/config"
	"brambleclaw/logger"
)

// SummaryNode represents a node in the hierarchical summary tree
type SummaryNode struct {
	ID         string    `json:"id"`
	Content    string    `json:"content"`
	Timestamp  time.Time `json:"timestamp"`
	Level      int       `json:"level"` // Hierarchy level (0 = leaf)
	ParentID   string    `json:"parent_id,omitempty"`
	ChildIDs   []string  `json:"child_ids,omitempty"`
	KeyContext []string  `json:"key_context,omitempty"` // Preserved decisions/actions
	TokenCount int       `json:"token_count"`
	Compressed bool      `json:"compressed"` // Whether this node has been compressed
}

// SummaryArchive represents archived summaries
type SummaryArchive struct {
	ArchivedAt  time.Time      `json:"archived_at"`
	NodeCount   int            `json:"node_count"`
	Nodes       []*SummaryNode `json:"nodes"`
	TotalTokens int            `json:"total_tokens"`
}

// HierarchyView for visualization/debugging
type HierarchyView struct {
	RootCount int         `json:"root_count"`
	MaxDepth  int         `json:"max_depth"`
	Nodes     []*NodeView `json:"nodes"`
}

// NodeView for visualization
type NodeView struct {
	ID       string      `json:"id"`
	Level    int         `json:"level"`
	Preview  string      `json:"preview"` // First 100 chars
	Children []*NodeView `json:"children,omitempty"`
}

// SummaryCompressor handles hierarchical summary compression
type SummaryCompressor struct {
	cfg       config.CompactConfig
	llmClient *LLMClient
	rootNodes []*SummaryNode
	nodeIndex map[string]*SummaryNode
	archives  []*SummaryArchive
}

// NewSummaryCompressor creates a new compressor instance
func NewSummaryCompressor(cfg config.CompactConfig, llmClient *LLMClient) *SummaryCompressor {
	return &SummaryCompressor{
		cfg:       cfg,
		llmClient: llmClient,
		rootNodes: make([]*SummaryNode, 0),
		nodeIndex: make(map[string]*SummaryNode),
		archives:  make([]*SummaryArchive, 0),
	}
}

// generateNodeID generates a unique ID for a node
func generateNodeID() string {
	bytes := make([]byte, 8)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

// AddSummary adds a new summary as a leaf node
func (sc *SummaryCompressor) AddSummary(content string, timestamp time.Time) (*SummaryNode, error) {
	if len(content) > sc.cfg.MaxSummaryLength {
		content = content[:sc.cfg.MaxSummaryLength]
	}

	node := &SummaryNode{
		ID:        generateNodeID(),
		Content:   content,
		Timestamp: timestamp,
		Level:     0,
		ChildIDs:  make([]string, 0),
	}

	if sc.cfg.PreserveKeyContext {
		node.KeyContext = sc.ExtractKeyContext(content)
	}

	sc.rootNodes = append(sc.rootNodes, node)
	sc.nodeIndex[node.ID] = node

	logger.L().Debug().
		Str("node_id", node.ID).
		Int("level", node.Level).
		Msg("Added summary node")

	// 如果启用了层级压缩，检查是否需要压缩当前层级
	if sc.cfg.EnableHierarchical {
		sc.compressLevelIfNeeded(node.Level)
	}

	return node, nil
}

// compressLevelIfNeeded 检查指定层级的节点数量，如果达到阈值则进行压缩
func (sc *SummaryCompressor) compressLevelIfNeeded(level int) {
	// 如果已达到最大深度，不再压缩
	if level >= sc.cfg.HierarchicalDepth {
		return
	}

	// 获取该层级的所有未压缩节点
	nodes := sc.getNodesAtLevel(level)
	if len(nodes) < 4 { // 每4个节点压缩成一个父节点
		return
	}

	// 取前4个节点进行压缩
	nodesToCompress := nodes[:4]

	// 创建父节点
	parent := &SummaryNode{
		ID:        generateNodeID(),
		Content:   "",
		Timestamp: time.Now(),
		Level:     level + 1,
		ChildIDs:  make([]string, 0, 4),
	}

	// 收集子节点内容
	var combinedContent strings.Builder
	for _, child := range nodesToCompress {
		parent.ChildIDs = append(parent.ChildIDs, child.ID)
		child.ParentID = parent.ID
		child.Compressed = true
		combinedContent.WriteString(child.Content)
		combinedContent.WriteString("\n\n")
	}

	// 生成压缩后的内容
	compressedContent, err := sc.generateCompressedSummary(combinedContent.String(), level+1)
	if err != nil {
		logger.L().Error().Err(err).Int("level", level).
			Msg("Failed to compress level, marking nodes as compressed anyway")
		parent.Content = combinedContent.String()
		if len(parent.Content) > sc.cfg.MaxSummaryLength {
			parent.Content = parent.Content[:sc.cfg.MaxSummaryLength]
		}
	} else {
		parent.Content = compressedContent
	}

	if sc.cfg.PreserveKeyContext {
		parent.KeyContext = sc.ExtractKeyContext(parent.Content)
	}

	// 将父节点添加到节点索引
	sc.nodeIndex[parent.ID] = parent

	// 从 rootNodes 中移除被压缩的节点，添加父节点
	sc.updateRootNodes(nodesToCompress, parent)

	logger.L().Debug().
		Str("parent_id", parent.ID).
		Int("level", parent.Level).
		Int("children_count", len(parent.ChildIDs)).
		Msg("Created compressed parent node from level compression")

	// 递归检查父节点所在层级是否需要进一步压缩
	sc.compressLevelIfNeeded(level + 1)
}

// getNodesAtLevel 获取指定层级的所有未压缩根节点
func (sc *SummaryCompressor) getNodesAtLevel(level int) []*SummaryNode {
	var nodes []*SummaryNode
	for _, node := range sc.rootNodes {
		if node.Level == level && !node.Compressed {
			nodes = append(nodes, node)
		}
	}
	return nodes
}

// updateRootNodes 从 rootNodes 中移除被压缩的节点，添加父节点
func (sc *SummaryCompressor) updateRootNodes(compressedNodes []*SummaryNode, parent *SummaryNode) {
	// 创建压缩节点ID集合，方便查找
	compressedIDs := make(map[string]bool)
	for _, node := range compressedNodes {
		compressedIDs[node.ID] = true
	}

	// 新的 rootNodes 列表
	var newRootNodes []*SummaryNode
	for _, node := range sc.rootNodes {
		if !compressedIDs[node.ID] {
			newRootNodes = append(newRootNodes, node)
		}
	}
	// 添加父节点
	newRootNodes = append(newRootNodes, parent)
	sc.rootNodes = newRootNodes
}

// CompressNode compresses a node and its children into a parent node
func (sc *SummaryCompressor) CompressNode(node *SummaryNode) (*SummaryNode, error) {
	if node.Compressed {
		return nil, fmt.Errorf("node %s is already compressed", node.ID)
	}

	children := make([]*SummaryNode, 0)
	for _, childID := range node.ChildIDs {
		if child, ok := sc.nodeIndex[childID]; ok {
			children = append(children, child)
		}
	}

	if len(children) == 0 {
		node.Compressed = true
		return node, nil
	}

	var combinedContent strings.Builder
	for _, child := range children {
		combinedContent.WriteString(child.Content)
		combinedContent.WriteString("\n\n")
	}

	compressedContent, err := sc.generateCompressedSummary(combinedContent.String(), node.Level+1)
	if err != nil {
		return nil, fmt.Errorf("failed to generate compressed summary: %w", err)
	}

	parent := &SummaryNode{
		ID:        generateNodeID(),
		Content:   compressedContent,
		Timestamp: time.Now(),
		Level:     node.Level + 1,
		ChildIDs:  []string{node.ID},
	}

	node.ParentID = parent.ID
	node.Compressed = true

	sc.nodeIndex[parent.ID] = parent

	for i, root := range sc.rootNodes {
		if root.ID == node.ID {
			sc.rootNodes[i] = parent
			break
		}
	}

	logger.L().Debug().
		Str("parent_id", parent.ID).
		Str("child_id", node.ID).
		Int("level", parent.Level).
		Msg("Created compressed parent node")

	return parent, nil
}

// generateCompressedSummary uses LLM to compress content
func (sc *SummaryCompressor) generateCompressedSummary(content string, level int) (string, error) {
	if sc.llmClient == nil {
		if len(content) > sc.cfg.MaxSummaryLength {
			return content[:sc.cfg.MaxSummaryLength] + "...", nil
		}
		return content, nil
	}

	req := ChatCompletionRequest{
		Model: "default",
		Messages: []ChatMsg{
			{
				Role:    "system",
				Content: fmt.Sprintf("Compress into concise summary at level %d. Preserve key decisions, actions, outcomes.", level),
			},
			{
				Role:    "user",
				Content: content,
			},
		},
	}

	resp, err := sc.llmClient.Chat(req)
	if err != nil {
		return "", fmt.Errorf("LLM compression failed: %w", err)
	}

	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("LLM returned no choices")
	}

	compressed := resp.Choices[0].Message.Content

	if len(compressed) > sc.cfg.MaxSummaryLength {
		compressed = compressed[:sc.cfg.MaxSummaryLength]
	}

	return compressed, nil
}

// ExtractKeyContext extracts decisions, actions, and outcomes from content
func (sc *SummaryCompressor) ExtractKeyContext(content string) []string {
	context := make([]string, 0)

	patterns := []string{
		`(?i)(decided?|decision)\s+(?:to\s+)?(.+?)(?:\.|$|\n)`,
		`(?i)(action|took|implemented?)\s*[:\s]+(.+?)(?:\.|$|\n)`,
		`(?i)(outcome|result|conclusion)\s*[:\s]+(.+?)(?:\.|$|\n)`,
		`(?i)(error|failure|issue|problem)\s*[:\s]+(.+?)(?:\.|$|\n)`,
		`(?i)(success|completed?|achieved?)\s*[:\s]+(.+?)(?:\.|$|\n)`,
	}

	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		matches := re.FindAllStringSubmatch(content, -1)
		for _, match := range matches {
			if len(match) >= 3 {
				context = append(context, strings.TrimSpace(match[2]))
			}
		}
	}

	return context
}

// Archive archives old summary nodes older than the cutoff time
func (sc *SummaryCompressor) Archive(cutoff time.Time) (*SummaryArchive, error) {
	nodesToArchive := make([]*SummaryNode, 0)
	remainingRoots := make([]*SummaryNode, 0)

	for _, node := range sc.rootNodes {
		if node.Timestamp.Before(cutoff) {
			nodesToArchive = append(nodesToArchive, node)
		} else {
			remainingRoots = append(remainingRoots, node)
		}
	}

	if len(nodesToArchive) == 0 {
		return nil, fmt.Errorf("no nodes to archive")
	}

	totalTokens := 0
	for _, node := range nodesToArchive {
		totalTokens += node.TokenCount
	}

	archive := &SummaryArchive{
		ArchivedAt:  time.Now(),
		NodeCount:   len(nodesToArchive),
		Nodes:       nodesToArchive,
		TotalTokens: totalTokens,
	}

	for _, node := range nodesToArchive {
		delete(sc.nodeIndex, node.ID)
	}

	sc.rootNodes = remainingRoots
	sc.archives = append(sc.archives, archive)

	logger.L().Debug().
		Int("archived_count", archive.NodeCount).
		Time("cutoff", cutoff).
		Msg("Archived old summary nodes")

	return archive, nil
}

// GetHierarchy returns the hierarchical structure for visualization
func (sc *SummaryCompressor) GetHierarchy() *HierarchyView {
	view := &HierarchyView{
		RootCount: len(sc.rootNodes),
		MaxDepth:  0,
		Nodes:     make([]*NodeView, 0),
	}

	for _, root := range sc.rootNodes {
		nodeView := sc.buildNodeView(root)
		view.Nodes = append(view.Nodes, nodeView)
		if nodeView.Level > view.MaxDepth {
			view.MaxDepth = nodeView.Level
		}
	}

	return view
}

// buildNodeView recursively builds NodeView from SummaryNode
func (sc *SummaryCompressor) buildNodeView(node *SummaryNode) *NodeView {
	view := &NodeView{
		ID:      node.ID,
		Level:   node.Level,
		Preview: node.Content,
	}

	if len(view.Preview) > 100 {
		view.Preview = view.Preview[:100] + "..."
	}

	view.Children = make([]*NodeView, 0)
	for _, childID := range node.ChildIDs {
		if child, ok := sc.nodeIndex[childID]; ok {
			childView := sc.buildNodeView(child)
			view.Children = append(view.Children, childView)
		}
	}

	return view
}

// BuildSessionSummary 按照层级逻辑（从高层到底层，从旧到新）组装最终摘要
func (sc *SummaryCompressor) BuildSessionSummary() string {
	if len(sc.rootNodes) == 0 {
		return ""
	}

	// 1. 将根节点按层级分组
	levelMap := make(map[int][]*SummaryNode)
	maxLevel := 0
	for _, node := range sc.rootNodes {
		levelMap[node.Level] = append(levelMap[node.Level], node)
		if node.Level > maxLevel {
			maxLevel = node.Level
		}
	}

	var fullSummary strings.Builder
	fullSummary.WriteString("### Hierarchical Session Summary ###\n")

	// 2. 从最高层级（最久远/最概括）向下遍历到 Level 0（最近/最详细）
	for l := maxLevel; l >= 0; l-- {
		nodes, exists := levelMap[l]
		if !exists || len(nodes) == 0 {
			continue
		}

		// 同一 Level 内，我们假设在 rootNodes 中的原始顺序即为添加顺序
		// 如果需要更精确，可以保留添加时的全局 Sequence ID
		for _, node := range nodes {
			// 根据层级添加缩进或前缀，帮助 LLM 理解结构
			prefix := strings.Repeat("  ", maxLevel-l)
			fullSummary.WriteString(fmt.Sprintf("%s* [Level %d]: %s\n", prefix, l, node.Content))

			if len(node.KeyContext) > 0 {
				fullSummary.WriteString(fmt.Sprintf("%s  Context: %s\n", prefix, strings.Join(node.KeyContext, " | ")))
			}
		}
	}

	return strings.TrimSpace(fullSummary.String())
}

// PrintHierarchy 将层级视图以树状结构打印到控制台
func (sc *SummaryCompressor) PrintHierarchy() {
	view := sc.GetHierarchy()
	fmt.Printf("Summary Hierarchy (Roots: %d, Max Depth: %d)\n", view.RootCount, view.MaxDepth)
	fmt.Println(strings.Repeat("=", 50))

	for i, node := range view.Nodes {
		isLast := i == len(view.Nodes)-1
		sc.renderNode(node, "", isLast)
	}
}

// renderNode 递归渲染节点及其子节点
func (sc *SummaryCompressor) renderNode(node *NodeView, indent string, isLast bool) {
	// 1. 处理预览内容：去掉换行符，让它只占一行
	cleanPreview := strings.ReplaceAll(node.Preview, "\n", " ")
	if len(cleanPreview) > 80 {
		cleanPreview = cleanPreview[:77] + "..."
	}

	// 2. 选择连接符
	marker := "├── "
	if isLast {
		marker = "└── "
	}

	// 3. 打印当前节点（增加颜色或高亮提示 Level 更好，这里用中括号区分）
	fmt.Printf("%s%s[Level %d] ID: %s | %s\n", indent, marker, node.Level, node.ID[:6], cleanPreview)

	// 4. 计算下一层的缩进
	newIndent := indent
	if isLast {
		newIndent += "    "
	} else {
		newIndent += "│   "
	}

	// 5. 递归打印子节点
	for i, child := range node.Children {
		sc.renderNode(child, newIndent, i == len(node.Children)-1)
	}
}
