package runtime

import (
	"context"
	"sync"

	"brambleclaw/internal/messages"
)

type AgentRuntime struct {
	mu     sync.RWMutex
	topics map[TopicID]*Topic
}

func NewAgentRuntime() *AgentRuntime {
	return &AgentRuntime{
		topics: make(map[TopicID]*Topic),
	}
}

func (r *AgentRuntime) GetTopic(id TopicID) *Topic {
	r.mu.Lock()
	defer r.mu.Unlock()

	if topic, ok := r.topics[id]; ok {
		return topic
	}
	topic := NewTopic(id)
	r.topics[id] = topic
	return topic
}

// Publish 满足 messages.RuntimeProvider 接口
func (r *AgentRuntime) Publish(ctx context.Context, topicID string, msg messages.BaseMessage) {
	topic := r.GetTopic(TopicID(topicID))
	topic.Publish(ctx, msg)
}

// Subscribe 满足 messages.RuntimeProvider 接口
func (r *AgentRuntime) Subscribe(ctx context.Context, topicID string) messages.RuntimeSubscription {
	topic := r.GetTopic(TopicID(topicID))
	return topic.Subscribe(ctx)
}

// PublishTo 按 TopicID 发布（非接口方法，供 runtime 包内部使用）
func (r *AgentRuntime) PublishTo(ctx context.Context, topicID TopicID, msg messages.BaseMessage) {
	topic := r.GetTopic(topicID)
	topic.Publish(ctx, msg)
}

// SubscribeTo 按 TopicID 订阅（非接口方法，供 runtime 包内部使用）
func (r *AgentRuntime) SubscribeTo(ctx context.Context, topicID TopicID) *Subscription {
	topic := r.GetTopic(topicID)
	return topic.Subscribe(ctx)
}

func (r *AgentRuntime) RemoveTopic(id TopicID) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if topic, ok := r.topics[id]; ok {
		topic.mu.Lock()
		for _, sub := range topic.subs {
			sub.cancel()
		}
		topic.mu.Unlock()
		delete(r.topics, id)
	}
}

func (r *AgentRuntime) Stop() {
	r.mu.Lock()
	defer r.mu.Unlock()

	for id, topic := range r.topics {
		topic.mu.Lock()
		for _, sub := range topic.subs {
			sub.cancel()
		}
		topic.mu.Unlock()
		delete(r.topics, id)
	}
}

func (r *AgentRuntime) TopicIDs() []TopicID {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ids := make([]TopicID, 0, len(r.topics))
	for id := range r.topics {
		ids = append(ids, id)
	}
	return ids
}

// 编译时检查：确保 AgentRuntime 实现了 RuntimeProvider 接口
var _ messages.RuntimeProvider = (*AgentRuntime)(nil)
