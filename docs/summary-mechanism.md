# Summary 机制设计

## 概述

Neoclaw 的 Summary 机制是一个层级化的对话摘要压缩系统，旨在解决大模型对话中的"长期记忆"与"上下文窗口限制"之间的矛盾。该系统不简单地丢弃旧消息，而是通过类似 B-树的分裂与合并机制，将摘要节点逐步向上压缩，实现高效的上下文管理。

## 核心组件

### 1. SummaryCompressor（摘要压缩器）

**文件位置**: `internal/agent/summary_compressor.go`

`SummaryCompressor` 是整个摘要机制的核心，负责管理层级化的摘要树结构。

#### 核心数据结构

```go
type SummaryCompressor struct {
    cfg       config.CompactConfig
    llmClient *LLMClient
    rootNodes []*SummaryNode
    nodeIndex map[string]*SummaryNode
    archives  []*SummaryArchive
}
```

- **cfg**: 压缩配置，控制压缩阈值、深度等参数
- **llmClient**: LLM 客户端，用于生成压缩后的摘要
- **rootNodes**: 当前所有未被进一步压缩的顶层节点
- **nodeIndex**: 全局索引，通过 ID 毫秒级检索任何节点
- **archives**: 已归档的历史摘要

#### SummaryNode（摘要节点）

```go
type SummaryNode struct {
    ID         string    `json:"id"`
    Content    string    `json:"content"`
    Timestamp  time.Time `json:"timestamp"`
    Level      int       `json:"level"`      // 层级（0 = 叶子）
    ParentID   string    `json:"parent_id,omitempty"`
    ChildIDs   []string  `json:"child_ids,omitempty"`
    KeyContext []string  `json:"key_context,omitempty"` // 保留的关键决策/行动
    TokenCount int       `json:"token_count"`
    Compressed bool      `json:"compressed"` // 是否已压缩
}
```

**字段说明**:

- **ID**: 节点唯一标识，8字节随机hex编码
- **Content**: 摘要内容
- **Timestamp**: 时间戳
- **Level**: 层级，0表示原始摘要，层级越高表示越抽象的概括
- **ParentID**: 父节点ID
- **ChildIDs**: 子节点ID列表
- **KeyContext**: 提取的关键上下文（决策、行动、结果等）
- **TokenCount**: Token数量
- **Compressed**: 是否已被压缩标记

### 2. CompactConfig（压缩配置）

**文件位置**: `internal/config/structs/compact.go`

```go
type CompactConfig struct {
    CompactThreshold    int  `json:"compact_threshold"`    // 触发压缩的Token阈值
    CompactRounds       int  `json:"compact_rounds"`      // 触发压缩的消息间隔
    MaxSummaryLength    int  `json:"max_summary_length"`  // 每个摘要的最大长度（默认10000）
    EnableHierarchical  bool `json:"enable_hierarchical"` // 启用层级化摘要
    HierarchicalDepth   int  `json:"hierarchical_depth"`  // 最大深度（默认3）
    ArchiveOldSummaries bool `json:"archive_old_summaries"` // 归档而非删除旧摘要
    PreserveKeyContext  bool `json:"preserve_key_context"` // 压缩时保留关键上下文
}
```

**默认配置**:

```go
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
```

### 3. ContextBuilder（上下文构建器）

**文件位置**: `internal/agent/context.go`

`ContextBuilder` 负责整合摘要系统与对话流程，是摘要机制的对外接口。

#### 核心方法


| 方法                  | 功能                     |
| --------------------- | ------------------------ |
| `Compact()`           | 按阈值自动触发压缩       |
| `ForceCompact()`      | 强制手动压缩（忽略阈值） |
| `GetSessionSummary()` | 获取当前会话摘要         |
| `ResetCompressor()`   | 重置压缩器状态           |
| `Build()`             | 构建完整的系统提示词     |

### 4. Session（会话管理）

**文件位置**: `internal/session/session.go`

会话管理记录摘要进度和元数据。

```go
type Session struct {
    Key               string
    Messages          []messages.BaseMessage
    CreatedAt         time.Time
    UpdatedAt         time.Time
    Summarized        int  // 指向最古老的有效信息（已摘要位置）
    Modified          bool
    LastSavedChecksum string
    // ...
}
```

### 5. SessionMetadata（会话元数据）

**文件位置**: `internal/session/session_meta.go`

```go
type SessionMetadata struct {
    AgentName        string
    ChannelName      string
    ChatID           string
    CreatedAt        time.Time
    UpdatedAt        time.Time
    MessageCount     int
    TokenCount       int
    SessionSummary   string  // 会话摘要
    FirstUserMessage string
}
```

## 工作原理

### 层级化压缩机制

系统采用类似 B-树的分裂与合并策略：

1. **初始状态**: 所有摘要都是 Level 0 的叶子节点，按时间顺序添加到 `rootNodes`
2. **触发压缩**: 当某一层级的节点数达到 4 个时，触发合并操作
3. **合并操作**:
   - 取最旧的 4 个节点
   - 使用 LLM 将它们的内容压缩为一个新的摘要
   - 创建新的父节点（Level + 1）
   - 设置子节点的 `ParentID` 和 `Compressed` 标记
   - 从 `rootNodes` 中移除这 4 个子节点，添加父节点
4. **递归压缩**: 检查父节点所在的层级是否也需要压缩

#### 可视化演变过程

**初始**: 3个节点

```
rootNodes: [L0-a, L0-b, L0-c]
```

**加入第4个，触发压缩**:

```
rootNodes: [L0-a, L0-b, L0-c, L0-d] → 压缩为 [L1-A]
```

**继续积累，形成多层结构**:

```
rootNodes: [L1-A, L1-B, L1-C, L0-x, L0-y]
```

### 压缩流程

#### 1. 触发条件

自动压缩触发条件（`Compact()` 方法）:

```
token使用量 > CompactThreshold && 消息数 % CompactRounds == 0
```

#### 2. 压缩批次

每次压缩 `CompactRounds / 4` 条消息（避免一次压缩过多）。

#### 3. 摘要生成

使用 LLM 生成摘要:

```go
req := ChatCompletionRequest{
    Model: "default",
    Messages: []ChatMsg{
        {
            Role: "system",
            Content: fmt.Sprintf("Compress into concise summary at level %d. Preserve key decisions, actions, outcomes.", level),
        },
        {
            Role: "user",
            Content: combinedContent,
        },
    },
}
```

#### 4. 关键上下文提取

系统使用正则表达式自动提取关键信息:

```go
patterns := []string{
    `(?i)(decided?|decision)\s+(?:to\s+)?(.+?)(?:\.|$|\n)`,      // 决策
    `(?i)(action|took|implemented?)\s*[:\s]+(.+?)(?:\.|$|\n)`,  // 行动
    `(?i)(outcome|result|conclusion)\s*[:\s]+(.+?)(?:\.|$|\n)`, // 结果
    `(?i)(error|failure|issue|problem)\s*[:\s]+(.+?)(?:\.|$|\n)`, // 问题
    `(?i)(success|completed?|achieved?)\s*[:\s]+(.+?)(?:\.|$|\n)`, // 成功
}
```

### 会话摘要构建

`BuildSessionSummary()` 按层级从高到低组装摘要:

```go
// 输出格式示例:
### Hierarchical Session Summary ###

  * [Level 2]: 长期对话的高度概括...
    Context: 决定重构架构 | 完成模块拆分

* [Level 1]: 中期对话的概括...
  Context: 实现了缓存系统 | 修复了内存泄漏

* [Level 0]: 近期对话的详细摘要...
  Context: 用户要求添加导出功能 | 已完成CSV导出实现
```

### 优势

1. **渐进式抽象**: 旧记忆被递归抽象为更高层级的浓缩文本
2. **近期详情**: 最近的记忆（低层级节点）保持更多细节
3. **高效检索**: 通过 `nodeIndex` 实现 O(1) 节点查找
4. **可控深度**: 通过 `HierarchicalDepth` 限制最大层级
5. **灵活配置**: 可调整压缩阈值、批次大小等参数

## 集成到 AgentManager

**文件位置**: `internal/agent/agent_manager.go`

在 Agent 初始化时设置摘要系统:

```go
// 1. 创建 ContextBuilder
contextBuilder, err := NewContextBuilder(&fullCfg.Compact)

// 2. 初始化 LLM 客户端
llmClient := NewLLMClient(fullCfg.LLMConfig)

// 3. 初始化 SummaryCompressor
contextBuilder.SetSummaryCompressor(NewSummaryCompressor(fullCfg.Compact, llmClient))

// 4. 设置 Agent 引用
contextBuilder.SetAgent(agent)
```

## 调试与可视化

### 层级视图（HierarchyView）

```go
type HierarchyView struct {
    RootCount int
    MaxDepth  int
    Nodes     []*NodeView
}

type NodeView struct {
    ID       string
    Level    int
    Preview  string  // 前100字符
    Children []*NodeView
}
```

### PrintHierarchy（打印层级）

`PrintHierarchy()` 方法将层级树以树状结构打印到日志:

```
[DEBUG] Summary node: level=2, id=abcd1234, preview="长期对话概括..."
  ├─ [DEBUG] Summary node: level=1, id=efgh5678, preview="中期摘要..."
  │   ├─ [DEBUG] Summary node: level=0, id=ijkl9012, preview="详细内容..."
  │   └─ ...
  └─ ...
```

## 归档机制

### SummaryArchive（摘要归档）

```go
type SummaryArchive struct {
    ArchivedAt time.Time
    NodeCount  int
    Nodes      []*SummaryNode
    TotalTokens int
}
```

当启用 `ArchiveOldSummaries` 时，超过时间阈值的节点会被归档而不是删除。

## 使用示例

### 1. 基本使用

```go
// 创建压缩器
compressor := NewSummaryCompressor(cfg, llmClient)

// 添加摘要
node, err := compressor.AddSummary("用户询问项目架构...", time.Now())

// 获取完整会话摘要
summary := compressor.BuildSessionSummary()
```

### 2. 手动触发压缩

```go
// 强制压缩所有未压缩消息（保留最后1条）
compressedCount, err := contextBuilder.ForceCompact(ctx, sess, info)

// 或按阈值自动压缩
err := contextBuilder.Compact(ctx, sess, info)
```

### 3. 配置调整

```go
cfg := config.CompactConfig{
    CompactThreshold:   8000,          // 提高阈值
    EnableHierarchical: true,          // 启用层级压缩
    HierarchicalDepth:  5,             // 增加最大深度
    PreserveKeyContext: true,          // 保留关键上下文
}
```

## 扩展点

### 自定义压缩策略

继承 `SummaryCompressor` 并重写 `compressLevelIfNeeded()` 方法可实现自定义压缩策略。

### 自定义关键上下文提取

重写 `ExtractKeyContext()` 方法可添加更多模式匹配规则。

### 插件式 LLM 压缩

当前使用内置 LLM 客户端，可扩展为支持多种压缩策略（如不同的提示词模板、不同的模型等）。

## 注意事项

1. **Token 消耗**: 压缩操作本身会消耗 Token，需权衡压缩频率与 Token 成本
2. **信息损失**: 层级越高，信息越抽象，可能丢失细节
3. **配置平衡**: 阈值太低会频繁压缩，太高则无法有效节省上下文
4. **持久化**: 当前版本仅将最终摘要存入 SessionMetadata，完整层级树未持久化
