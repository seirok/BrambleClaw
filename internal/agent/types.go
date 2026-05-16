package agent

import (
	"brambleclaw/internal/interfaces"
	"brambleclaw/internal/messages"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Role 消息角色
type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleSystem    Role = "system"
	RoleTool      Role = "tool"
)

type ChatMsg struct {
	Role       Role       `json:"role"`
	Content    string     `json:"content"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
}

type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function ToolFunction `json:"function"`
}

// UnmarshalJSON 自定义反序列化，确保 type 字段默认为 "function"
func (tc *ToolCall) UnmarshalJSON(data []byte) error {
	type Alias ToolCall
	var aux Alias
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	*tc = ToolCall(aux)
	if tc.Type == "" {
		tc.Type = "function"
	}
	return nil
}

type ToolFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type ToolResult struct {
	Content string
	CallId  string
}

type ChatCompletionRequest struct {
	Model    string                   `json:"model"`
	Messages []ChatMsg                `json:"messages"`
	Tools    []map[string]interface{} `json:"tools,omitempty"`
}

type LLMResponse struct {
	ID      string `json:"id"`
	Choices []struct {
		Message struct {
			Role      string     `json:"role"`
			Content   string     `json:"content"`
			ToolCalls []ToolCall `json:"tool_calls"`
		} `json:"message"`
	} `json:"choices"`
	Usage struct {
		CompletionTokens int `json:"completion_tokens"`
		PromptTokens     int `json:"prompt_tokens"`
	} `json:"usage"`
}

// ContentBlock 内容块接口
type ContentBlock interface {
	Type() string
}

// TextContent 文本内容
type TextContent struct {
	Text string `json:"text"`
}

func (t TextContent) Type() string {
	return "text"
}

// AgentMessage Agent消息
type AgentMessage struct {
	ID        string               `json:"id"`
	Source    string               `json:"source"`
	Type      messages.MessageType `json:"type"`
	Role      Role                 `json:"role"`
	Content   []ContentBlock       `json:"content"`
	Metadata  map[string]string    `json:"metadata,omitempty"`
	CreatedAt time.Time            `json:"created_at"`
}

// serializableContent 用于JSON序列化的内容块
type serializableContent struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

// MarshalJSON 自定义JSON序列化
func (m AgentMessage) MarshalJSON() ([]byte, error) {
	type Alias AgentMessage
	aux := &struct {
		*Alias
		Content []serializableContent `json:"content"`
	}{
		Alias: (*Alias)(&m),
	}

	// 序列化 Content
	for _, block := range m.Content {
		var data []byte
		var err error
		switch v := block.(type) {
		case TextContent:
			data, err = json.Marshal(v)
		default:
			data, err = json.Marshal(block)
		}
		if err != nil {
			return nil, err
		}
		aux.Content = append(aux.Content, serializableContent{
			Type: block.Type(),
			Data: data,
		})
	}

	return json.Marshal(aux)
}

// NewAgentMessage 创建 AgentMessage（自动填充 ID/CreatedAt/Type）
func NewAgentMessage(source string, role Role, content string) *AgentMessage {
	return &AgentMessage{
		ID:        uuid.New().String(),
		Source:    source,
		Type:      messages.MessageTypeText,
		Role:      role,
		Content:   []ContentBlock{TextContent{Text: content}},
		Metadata:  make(map[string]string),
		CreatedAt: time.Now().UTC(),
	}
}

// BaseMessage 接口实现
func (m *AgentMessage) GetID() string                  { return m.ID }
func (m *AgentMessage) GetSource() string              { return m.Source }
func (m *AgentMessage) GetType() messages.MessageType  { return m.Type }
func (m *AgentMessage) GetCreatedAt() time.Time        { return m.CreatedAt }
func (m *AgentMessage) GetMetadata() map[string]string { return m.Metadata }
func (m *AgentMessage) GetRole() string                  { return string(m.Role) }
func (m *AgentMessage) ToText() string {
	var sb strings.Builder
	for _, ct := range m.Content {
		if tc, ok := ct.(TextContent); ok {
			sb.WriteString(tc.Text)
		}
	}
	return sb.String()
}

// 编译时检查
var _ messages.BaseMessage = (*AgentMessage)(nil)

// UnmarshalJSON 自定义JSON反序列化
func (m *AgentMessage) UnmarshalJSON(data []byte) error {
	type Alias AgentMessage
	aux := &struct {
		*Alias
		Content []serializableContent `json:"content"`
	}{
		Alias: (*Alias)(m),
	}

	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}

	// 反序列化 Content
	for _, sc := range aux.Content {
		switch sc.Type {
		case "text":
			var tc TextContent
			if err := json.Unmarshal(sc.Data, &tc); err != nil {
				return err
			}
			m.Content = append(m.Content, tc)
		default:
			// 未知类型，尝试作为 TextContent
			var tc TextContent
			if err := json.Unmarshal(sc.Data, &tc); err == nil {
				m.Content = append(m.Content, tc)
			}
		}
	}

	return nil
}

var (
	ErrAlreadyExists = errors.New("agent already exists")
	ErrNotFound      = errors.New("agent not found")
)

// 编译时检查：确保 AgentRegistry 实现了 Registry[Agent]
var _ interfaces.Registry[*Agent] = (*AgentRegistry)(nil)

type AgentRegistry struct {
	agents map[string]*Agent
	mu     sync.RWMutex
}

// NewAgentRegistry 创建具象注册表
func NewAgentRegistry() *AgentRegistry {
	return &AgentRegistry{
		agents: make(map[string]*Agent),
	}
}

// Register 注册 Agent
func (r *AgentRegistry) Register(ctx context.Context, name string, ag *Agent) error {
	if name == "" {
		return errors.New("agent name cannot be empty")
	}
	if ag == nil {
		return errors.New("agent instance cannot be nil")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.agents[name]; exists {
		return fmt.Errorf("%w: %s", ErrAlreadyExists, name)
	}

	r.agents[name] = ag
	return nil
}

// Get 获取 Agent
func (r *AgentRegistry) Get(ctx context.Context, name string) (*Agent, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ag, ok := r.agents[name]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrNotFound, name)
	}
	return ag, nil
}

// Unregister 注销 Agent
func (r *AgentRegistry) Unregister(ctx context.Context, name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.agents[name]; !exists {
		return fmt.Errorf("%w: %s", ErrNotFound, name)
	}

	delete(r.agents, name)
	return nil
}

// Update 更新 Agent
func (r *AgentRegistry) Update(ctx context.Context, name string, ag *Agent) error {
	if ag == nil {
		return errors.New("agent instance cannot be nil")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.agents[name]; !exists {
		return fmt.Errorf("%w: %s", ErrNotFound, name)
	}

	r.agents[name] = ag
	return nil
}

// List 返回所有 Agent
func (r *AgentRegistry) List(ctx context.Context) []*Agent {
	r.mu.RLock()
	defer r.mu.RUnlock()

	list := make([]*Agent, 0, len(r.agents))
	for _, ag := range r.agents {
		list = append(list, ag)
	}
	return list
}
