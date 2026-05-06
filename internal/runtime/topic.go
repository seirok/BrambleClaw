package runtime

import (
	"context"
	"sync"

	"brambleclaw/internal/messages"

	"github.com/google/uuid"
)

type TopicID string

type Subscription struct {
	id     string
	topic  TopicID
	ch     chan messages.BaseMessage
	cancel context.CancelFunc
}

func (s *Subscription) ID() string                      { return s.id }
func (s *Subscription) Topic() TopicID                  { return s.topic }
func (s *Subscription) Ch() <-chan messages.BaseMessage { return s.ch }
func (s *Subscription) Cancel()                         { s.cancel() }

type Topic struct {
	id   TopicID
	mu   sync.RWMutex
	subs map[string]*Subscription
}

func NewTopic(id TopicID) *Topic {
	return &Topic{
		id:   id,
		subs: make(map[string]*Subscription),
	}
}

func (t *Topic) ID() TopicID { return t.id }

func (t *Topic) Subscribe(ctx context.Context) *Subscription {
	t.mu.Lock()
	defer t.mu.Unlock()

	subCtx, cancel := context.WithCancel(ctx)
	sub := &Subscription{
		id:     uuid.NewString(),
		topic:  t.id,
		ch:     make(chan messages.BaseMessage, 100),
		cancel: cancel,
	}
	t.subs[sub.id] = sub

	go func() {
		<-subCtx.Done()
		t.mu.Lock()
		delete(t.subs, sub.id)
		close(sub.ch)
		t.mu.Unlock()
	}()

	return sub
}

func (t *Topic) Publish(ctx context.Context, msg messages.BaseMessage) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	for _, sub := range t.subs {
		select {
		case sub.ch <- msg:
		case <-ctx.Done():
			return
		default:
		}
	}
}

func (t *Topic) SubscriberCount() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return len(t.subs)
}
