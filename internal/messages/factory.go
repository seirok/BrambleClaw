package messages

import (
	"encoding/json"
	"fmt"
)

// MessageFactory 消息工厂，基于注册表的反序列化
type MessageFactory struct {
	registry map[MessageType]func() ChatMessage
}

// NewMessageFactory 创建消息工厂
func NewMessageFactory() *MessageFactory {
	f := &MessageFactory{
		registry: make(map[MessageType]func() ChatMessage),
	}
	f.Register(MessageTypeText, func() ChatMessage { return &TextMessage{} })
	f.Register(MessageTypeStop, func() ChatMessage { return &StopMessage{} })
	f.Register(MessageTypeHandoff, func() ChatMessage { return &HandoffMessage{} })
	f.Register(MessageTypeError, func() ChatMessage { return &AgentErrorMessage{} })
	return f
}

// Register 注册消息类型构造器
func (f *MessageFactory) Register(msgType MessageType, constructor func() ChatMessage) {
	f.registry[msgType] = constructor
}

// Unmarshal 两阶段反序列化：先读取 type 字段，再根据注册表创建具体类型
func (f *MessageFactory) Unmarshal(data []byte) (ChatMessage, error) {
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("messages: first-pass unmarshal failed: %w", err)
	}

	typeVal, ok := raw["type"]
	if !ok {
		return nil, fmt.Errorf("messages: missing 'type' field")
	}

	msgType := MessageType(fmt.Sprintf("%v", typeVal))
	constructor, ok := f.registry[msgType]
	if !ok {
		return nil, fmt.Errorf("messages: unregistered message type: %s", msgType)
	}

	msg := constructor()
	if err := json.Unmarshal(data, msg); err != nil {
		return nil, fmt.Errorf("messages: second-pass unmarshal for %s failed: %w", msgType, err)
	}

	return msg, nil
}
