package bus

import (
	"brambleclaw/logger"
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
)

type InBoundMessage struct {
	ID        string `json:"id"`
	SenderID  string `json:"sender_id"` // 发送者ID
	ChatID    string
	InChannel string // 消息来源Channel
	Content   string
	TimeStamp time.Time
}

type OutBoundMessage struct {
	ChatID     string `json:"chat_id"` // 聊天ID
	OutChannel string
	Content    string
	ReplyTo    string `json:"reply_to"` // 回复的消息ID
	TimeStamp  time.Time
}

func (m *InBoundMessage) SessionKey() string {
	return m.InChannel + ":" + m.ChatID
}

type MessageBus struct {
	InBound  chan *InBoundMessage
	OutBound chan *OutBoundMessage
	outSubs  map[string]chan *OutBoundMessage
	mu       sync.RWMutex
}

type MessageSubscription struct {
	ID      string
	Channel chan *OutBoundMessage
}

func NewMessageBus(buf_size int) *MessageBus {
	return &MessageBus{
		InBound:  make(chan *InBoundMessage, buf_size),
		OutBound: make(chan *OutBoundMessage, buf_size),
		outSubs:  make(map[string]chan *OutBoundMessage),
	}
}

// 在goclaw系统中，同一条消息可能需要被多个组件处理：
//
//- 通道管理器 ：需要将消息发送到对应的通道（如Telegram、CLI等）
//- 日志系统 ：需要记录所有消息
//- 监控系统 ：需要统计消息流量和处理状态
//- 其他组件 ：可能有其他组件需要处理特定类型的消息

func (mb *MessageBus) Subscribe() *MessageSubscription {
	id := uuid.New().String()
	channel := make(chan *OutBoundMessage, 100)

	mb.mu.Lock()
	mb.outSubs[id] = channel
	mb.mu.Unlock()

	return &MessageSubscription{
		ID:      id,
		Channel: channel,
	}
}

func (mb *MessageBus) Unsubscribe(id string) {
	mb.mu.Lock()
	delete(mb.outSubs, id)
	mb.mu.Unlock()
}

func (mb *MessageBus) DistributeOutBoundMessage(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-mb.OutBound:
			if !ok {
				logger.L().Debug().Msg("OutBound channel closed")
				return
			}

			mb.mu.RLock()
			// 非阻塞发送给所有订阅者
			for sub, ch := range mb.outSubs {
				select {
				case ch <- msg:
				case <-ctx.Done():
					mb.mu.RUnlock()
					return
				default:
					// 如果订阅者通道已满，可以选择跳过或记录警告
					logger.L().Debug().Str("Subscriber", sub).Msg("Subscriber channel full, dropping message")
				}
			}
			mb.mu.RUnlock()
		}
	}
}
func (mb *MessageBus) PublishInBoundMessage(ctx context.Context, in_msg *InBoundMessage) error {
	select {
	case mb.InBound <- in_msg:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}

}

func (mb *MessageBus) PublishOutBoundMessage(ctx context.Context, out_msg *OutBoundMessage) error {
	select {
	case mb.OutBound <- out_msg:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (mb *MessageBus) ConsumeInBoundMessage(ctx context.Context) (*InBoundMessage, error) {
	select {
	case msg := <-mb.InBound:
		logger.L().Debug().Str("Message received", msg.Content).Msg("")
		return msg, nil
	case <-ctx.Done():
		logger.L().Debug().Msg("Context cancelled")
		return nil, ctx.Err()
	}
}
