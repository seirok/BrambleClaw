package session

import (
	"context"
	"encoding/json"
	"fmt"
	util "neoclaw/internal"
	"neoclaw/internal/logger"
	"neoclaw/internal/messages"
	"neoclaw/internal/store"
	"os"
	"time"
)

const maxFirstUserMessageLen = 20

// messageUnmarshaller 反序列化 JSON 原始数据为 BaseMessage
type messageUnmarshaller func(json.RawMessage) (messages.BaseMessage, error)

// messageRegistry 全局消息类型注册表，避免 session → agent 循环依赖
var messageRegistry struct {
	unmarshallers map[string]messageUnmarshaller
}

func init() {
	messageRegistry.unmarshallers = make(map[string]messageUnmarshaller)
}

// RegisterMessageType 注册消息类型反序列化器，由 agent 包 init 时调用
func RegisterMessageType(typeName string, unmarshaller messageUnmarshaller) {
	messageRegistry.unmarshallers[typeName] = unmarshaller
}

func truncateWithEllipsis(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

type Session struct {
	Key               string                 `json:"key"`
	Messages          []messages.BaseMessage `json:"messages"` // 会话消息
	CreatedAt         time.Time              `json:"created_at"`
	UpdatedAt         time.Time              `json:"updated_at"`
	Summarized        int                    `json:"summarized"`          // 指向最古老的有效信息
	Modified          bool                   `json:"modified"`            // 自上次保存后是否修改过
	LastSavedChecksum string                 `json:"last_saved_checksum"` // 上次保存时的校验和，用于检测变化

	// 存储相关字段（非持久化）
	store     *store.FileStorage[Session]
	metaStore *store.FileStorage[SessionMetadata]
	meta      *SessionMetadata
}

// Option 定义 Session 的 functional option 类型
type Option func(*Session)

func WithKey(key string) Option {
	return func(s *Session) {
		s.Key = key
	}
}

// WithMessages 设置初始消息列表
func WithMessages(messages []messages.BaseMessage) Option {
	return func(s *Session) {
		s.Messages = messages
	}
}

// WithCreatedAt 设置创建时间
func WithCreatedAt(t time.Time) Option {
	return func(s *Session) {
		s.CreatedAt = t
	}
}

// WithUpdatedAt 设置更新时间
func WithUpdatedAt(t time.Time) Option {
	return func(s *Session) {
		s.UpdatedAt = t
	}
}

// WithSummarized 设置已摘要位置
func WithSummarized(n int) Option {
	return func(s *Session) {
		s.Summarized = n
	}
}

// WithModified 设置修改状态
func WithModified(modified bool) Option {
	return func(s *Session) {
		s.Modified = modified
	}
}

// WithLastSavedChecksum 设置上次保存的校验和
func WithLastSavedChecksum(checksum string) Option {
	return func(s *Session) {
		s.LastSavedChecksum = checksum
	}
}

// WithMetadata 设置 metadata
func WithMetadata(meta *SessionMetadata) Option {
	return func(s *Session) {
		s.meta = meta
	}
}

// WithStores 直接设置 store 和 metaStore（主要用于测试和 Manager 注入）
func WithStores(store *store.FileStorage[Session], metaStore *store.FileStorage[SessionMetadata]) Option {
	return func(s *Session) {
		s.store = store
		s.metaStore = metaStore
	}
}

// NewSession 创建新的 Session 实例（使用 functional options 模式）
func NewSession(opts ...Option) *Session {
	s := &Session{
		Key:       "",
		Messages:  []messages.BaseMessage{},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		store:     store.NewFileStorage[Session](""),
		metaStore: store.NewFileStorage[SessionMetadata](""),
		meta: &SessionMetadata{
			ChatID: util.GenerateRandomChatID(),
		},
	}

	// 应用所有选项
	for _, opt := range opts {
		opt(s)
	}

	return s
}

// Name 返回 session 名称（Service 接口）
func (s *Session) Name() string {
	return s.Key
}

// UnmarshalJSON 自定义反序列化，处理 Messages 接口切片
func (s *Session) UnmarshalJSON(data []byte) error {
	type Alias Session
	aux := &struct {
		Messages []json.RawMessage `json:"messages"`
		*Alias
	}{
		Alias: (*Alias)(s),
	}

	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}

	s.Messages = make([]messages.BaseMessage, 0, len(aux.Messages))
	for _, raw := range aux.Messages {
		// 探测消息类型字段
		var hint struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(raw, &hint); err != nil {
			return fmt.Errorf("failed to detect message type: %w", err)
		}

		unmarshaller, ok := messageRegistry.unmarshallers[hint.Type]
		if !ok {
			return fmt.Errorf("unknown message type: %s", hint.Type)
		}

		msg, err := unmarshaller(raw)
		if err != nil {
			return fmt.Errorf("failed to unmarshal message of type %s: %w", hint.Type, err)
		}
		s.Messages = append(s.Messages, msg)
	}

	return nil
}

// Start 从磁盘加载 session 和 metadata
func (s *Session) Load(ctx context.Context) error {
	// 加载 session 数据
	sessionFile := util.GetSessionFile(util.SessionKeyToFile(s.Key))
	if session, err := s.store.Load(ctx, sessionFile); err == nil {
		s.Messages = session.Messages
		s.CreatedAt = session.CreatedAt
		s.UpdatedAt = session.UpdatedAt
		s.Summarized = session.Summarized
		s.Modified = false
		s.LastSavedChecksum = session.LastSavedChecksum
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("加载 session 失败(%s): %w", s.Key, err)
	}

	// 加载 metadata
	metaFile := util.GetSessionMetaFile(util.SessionKeyToFile(s.Key))
	if meta, err := s.metaStore.Load(ctx, metaFile); err == nil {
		s.meta = meta
	} else if os.IsNotExist(err) {
		// 如果不存在，创建新的 metadata
		agentName, channelName, chatID, err := util.ParseSessionKey(s.Key)
		if err != nil {
			return fmt.Errorf("解析 session key 失败(%s): %w", s.Key, err)
		}
		s.meta = &SessionMetadata{
			AgentName:   agentName,
			ChannelName: channelName,
			ChatID:      chatID,
			CreatedAt:   time.Now(),
		}
	} else {
		return fmt.Errorf("加载 session meta 失败(%s): %w", s.Key, err)
	}

	// 自动补全 FirstUserMessage
	if s.meta.FirstUserMessage == "" {
		for _, msg := range s.Messages {
			if msg.GetSource() == "user" {
				s.meta.FirstUserMessage = truncateWithEllipsis(msg.ToText(), maxFirstUserMessageLen)
				// 标记需要保存
				s.Modified = true
				break
			}
		}
	}

	return nil
}

// Stop 将 session 和 metadata 保存到磁盘
func (s *Session) Save(ctx context.Context) error {
	// 保存 session 数据
	if !s.IsValid() {
		return nil
	} // 仅持久化有对话内容的 session

	if err := s.store.Save(ctx, util.GetSessionFile(util.SessionKeyToFile(s.Key)), s); err != nil {
		return fmt.Errorf("保存 session 失败(%s): %w", s.Key, err)
	}

	// 更新并保存 metadata
	if s.meta != nil {
		s.meta.UpdatedAt = time.Now()
		s.meta.MessageCount = len(s.Messages)
		// 回填 FirstUserMessage
		if s.meta.FirstUserMessage == "" {
			for _, msg := range s.Messages {
				if msg.GetSource() == "user" {
					s.meta.FirstUserMessage = truncateWithEllipsis(msg.ToText(), maxFirstUserMessageLen)
					break
				}
			}
		}
		if err := s.metaStore.Save(ctx, util.GetSessionMetaFile(util.SessionKeyToFile(s.Key)), s.meta); err != nil {
			return fmt.Errorf("保存 session meta 失败(%s): %w", s.Key, err)
		}
	}

	logger.L().Debug().Str("session file", util.GetSessionMetaFile(util.SessionKeyToFile(s.Key))).Msg("session has been saved")
	s.Modified = false
	return nil
}

func (s *Session) Get(idx int) (messages.BaseMessage, error) {
	// TODO: 完善合法性检查
	return s.Messages[idx], nil
}

func (s *Session) Append(message messages.BaseMessage) {
	s.Messages = append(s.Messages, message)
	s.Modified = true
}

func (s *Session) Remove(idx int) error {
	// TODO: 合法性检查
	s.Messages = append(s.Messages[:idx], s.Messages[idx+1:]...)
	return nil
}

// Len 返回消息列表长度
func (s *Session) Len() int {
	return len(s.Messages)
}

func (s *Session) Reset() {
	// TODO
}

// Clear 清空前持久化，清空后使用新的ChatID
func (s *Session) Clear(ctx context.Context) error {
	err := s.Save(ctx)
	if err != nil {
		return err
	}

	s.Messages = []messages.BaseMessage{}
	s.Modified = false

	// 采用新的session key
	an, cn, _, err := util.ParseSessionKey(s.Key)
	if err != nil {
		return err
	}
	cid := util.GenerateRandomChatID()
	s.Key = util.BuildSessionKey(an, cn, cid)

	// 新的 meta data
	s.meta = NewSessionMetadata(WithAgentName(an),
		WithChannelName(cn),
		WithChatID(cid),
	)

	s.CreatedAt = time.Now()
	s.UpdatedAt = time.Now()
	s.LastSavedChecksum = ""

	return nil
}

func (s *Session) Update() {
	s.Modified = true
}

// GetMetadata 获取 session 的 metadata
func (s *Session) GetMetadata() *SessionMetadata {
	return s.meta
}

func (s *Session) LoadHistory() []messages.BaseMessage {
	if s.Summarized >= len(s.Messages) {
		return []messages.BaseMessage{}
	}
	return s.Messages[s.Summarized:]
}

func (s *Session) GetFirstUserMessage() string {
	if len(s.Messages) < 2 {
		logger.L().Debug().Msg("no valid user message")
		return ""
	}

	return s.Messages[1].ToText()
}

func (s *Session) UpdateSessionSummary(summary string) error {
	meta := s.GetMetadata()
	meta.SessionSummary = summary

	s.meta.UpdatedAt = time.Now()
	s.Update()
	return nil
}

func (s *Session) IsValid() bool {
	// 有消息的开始状态： system + user --> llm
	if len(s.Messages) == 0 {
		return false
	}
	return true
}

func (s *Session) ParseKeyToSetMeta() error {
	if s.Key == "" {
		return fmt.Errorf("Empty session key")
	}

	an, cn, cid, err := util.ParseSessionKey(s.Key)
	if err != nil {
		return err
	}

	WithChatID(cid)(s.meta)
	WithAgentName(an)(s.meta)
	WithChannelName(cn)(s.meta)
	return nil
}
