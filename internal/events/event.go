package events

import (
	"sync"
	"time"
)

// ThinkingEvent 表示一个思考过程事件
type ThinkingEvent struct {
	Point     string    // hook point 名称
	Timestamp time.Time // 事件时间戳
	Category  string    // "LLM" / "TOOL" / "AGENT" / "MSG" / "SANDBOX"
	Summary   string    // 人类可读摘要
	Detail    string    // 详细信息（verbosity=detail 时使用）
	Data      any       // 原始 hook 数据引用
}

// EventBus 事件总线，用于将 hook 事件转发到 TUI
type EventBus struct {
	ch     chan ThinkingEvent
	mu     sync.RWMutex
	closed bool
}

// NewEventBus 创建新的事件总线
func NewEventBus(bufferSize int) *EventBus {
	if bufferSize <= 0 {
		bufferSize = 200
	}
	return &EventBus{
		ch: make(chan ThinkingEvent, bufferSize),
	}
}

// Publish 发布事件（非阻塞）
// channel 满时丢弃事件而非阻塞 hook pipeline
func (eb *EventBus) Publish(event ThinkingEvent) {
	eb.mu.RLock()
	if eb.closed {
		eb.mu.RUnlock()
		return
	}
	select {
	case eb.ch <- event:
	default:
		// channel 满，尝试丢弃最旧的一个再发送
		select {
		case <-eb.ch:
		default:
		}
		select {
		case eb.ch <- event:
		default:
		}
	}
	eb.mu.RUnlock()
}

// Subscribe 订阅事件
func (eb *EventBus) Subscribe() <-chan ThinkingEvent {
	return eb.ch
}

// Close 关闭事件总线
func (eb *EventBus) Close() {
	eb.mu.Lock()
	if !eb.closed {
		eb.closed = true
		close(eb.ch)
	}
	eb.mu.Unlock()
}
