package messages

import (
	"fmt"
	"strings"
	"time"
)

// OutBoundData 出站消息数据，供调用方构造 bus.OutBoundMessage
type OutBoundData struct {
	ChatID    string
	Channel   string
	Content   string
	MsgType   string
	ReplyTo   string
	TimeStamp time.Time
}

// FromInBoundData 入站消息数据，从 bus.InBoundMessage 提取
type InBoundData struct {
	ID        string
	SenderID  string
	ChatID    string
	InChannel string
	Content   string
}

// FromInBoundData 将入站消息数据转换为 TextMessage
func FromInBoundData(in InBoundData) *TextMessage {
	msg := NewTextMessage(in.SenderID, in.Content)
	msg.Metadata["channel"] = in.InChannel
	msg.Metadata["chat_id"] = in.ChatID
	return msg
}

// ToOutBoundData 将 ChatMessage 转换为出站消息数据
func ToOutBoundData(msg ChatMessage, chatID, channel, replyTo string) OutBoundData {
	return OutBoundData{
		ChatID:    chatID,
		Channel:   channel,
		Content:   msg.ToText(),
		MsgType:   string(msg.GetType()),
		ReplyTo:   replyTo,
		TimeStamp: time.Now(),
	}
}

// IsStopMessage 检查消息是否为停止消息
func IsStopMessage(msg ChatMessage) bool {
	_, ok := msg.(*StopMessage)
	return ok
}

// IsHandoffMessage 检查消息是否为移交消息
func IsHandoffMessage(msg ChatMessage) bool {
	_, ok := msg.(*HandoffMessage)
	return ok
}

// GetHandoffTarget 获取移交目标，如果不是移交消息返回空字符串
func GetHandoffTarget(msg ChatMessage) string {
	if handoff, ok := msg.(*HandoffMessage); ok {
		return handoff.Target
	}
	return ""
}

// IsErrorMessage 检查消息是否为Agent错误消息
func IsErrorMessage(msg ChatMessage) bool {
	_, ok := msg.(*AgentErrorMessage)
	return ok
}

// GetErrorDetail 获取错误详情，如果不是错误消息返回空字符串
func GetErrorDetail(msg ChatMessage) string {
	if errMsg, ok := msg.(*AgentErrorMessage); ok {
		return errMsg.Error
	}
	return ""
}

// ChatMessageFromText 便捷函数：从文本创建 ChatMessage
func ChatMessageFromText(source, content string) ChatMessage {
	return NewTextMessage(source, content)
}

// FormatMessageList 格式化消息列表为文本摘要
func FormatMessageList(msgs []ChatMessage) string {
	lines := make([]string, 0, len(msgs))
	for _, msg := range msgs {
		lines = append(lines, fmt.Sprintf("[%s] %s: %s", msg.GetType(), msg.GetSource(), msg.ToText()))
	}
	return strings.Join(lines, "\n")
}
