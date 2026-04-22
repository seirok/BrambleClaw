package agent

import (
	"encoding/json"
	"time"
)

// Role 消息角色
type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleSystem    Role = "system"
	RoleTool      Role = "tool"
)

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
	Role      Role           `json:"role"`
	Content   []ContentBlock `json:"content"`
	Timestamp int64          `json:"timestamp"`
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

// Session 会话
type Session struct {
	Key               string         `json:"key"`
	Messages          []AgentMessage `json:"messages"` // 会话消息
	CreatedAt         time.Time      `json:"created_at"`
	UpdatedAt         time.Time      `json:"updated_at"`
	Summarized        int            `json:"summarized"`          // 指向最古老的有效信息
	Modified          bool           `json:"modified"`            // 自上次保存后是否修改过
	LastSavedChecksum string         `json:"last_saved_checksum"` // 上次保存时的校验和，用于检测变化
}

type SessionMetadata struct {
	AgentName    string    `json:"agent_name"`
	ChannelName  string    `json:"channel_name"`
	ChatID       string    `json:"chat_id"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	MessageCount int       `json:"message_count"`
	TokenCount   int       `json:"token_count"`
}

func (s *Session) LoadHistory() []AgentMessage {
	// TODO: 防御性编程
	return s.Messages[s.Summarized:]
}

func (s *Session) AddMessage(msg AgentMessage) {
	s.Messages = append(s.Messages, msg)
	s.Modified = true
}
