package httpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"neoclaw/internal/bus"
	"neoclaw/internal/channel"
	"neoclaw/internal/interfaces"

	"github.com/google/uuid"
)

// WebConnection represents a single SSE connection
type WebConnection struct {
	ChatID  string
	FlushCh chan []byte
	DoneCh  chan struct{}
}

// WebChannel implements channel.BaseChannel for web SSE
type WebChannel struct {
	*channel.BaseChannelImpl
	running     bool
	config      *channel.BaseChannelConfig
	msgBus      *bus.MessageBus
	subID       string
	connections map[string]*WebConnection // chatID -> connection
	mu          sync.RWMutex
}

// NewWebChannel creates a new WebChannel
func NewWebChannel(msgBus *bus.MessageBus) *WebChannel {
	return &WebChannel{
		BaseChannelImpl: channel.NewBaseChannelImpl("web", &channel.BaseChannelConfig{
			Enabled:    true,
			AllowedIDs: []string{},
		}, msgBus),
		running:     false,
		config:      &channel.BaseChannelConfig{Enabled: true, AllowedIDs: []string{}},
		msgBus:      msgBus,
		connections: make(map[string]*WebConnection),
	}
}

// Start starts the channel
func (w *WebChannel) Start(ctx context.Context) error {
	w.running = true
	sub := w.msgBus.Subscribe()
	w.subID = sub.ID

	// Route messages to connections
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case msg, ok := <-sub.Channel:
				if !ok {
					return
				}
				w.routeMessage(msg)
			}
		}
	}()

	return nil
}

// Stop stops the channel
func (w *WebChannel) Stop(ctx context.Context) error {
	w.running = false
	w.msgBus.Unsubscribe(w.subID)

	w.mu.Lock()
	for _, conn := range w.connections {
		close(conn.DoneCh)
	}
	w.connections = make(map[string]*WebConnection)
	w.mu.Unlock()

	return nil
}

// Send sends a message to the connected SSE client
func (w *WebChannel) Send(ctx context.Context, msg *bus.OutBoundMessage) error {
	w.mu.RLock()
	conn, ok := w.connections[msg.ChatID]
	w.mu.RUnlock()

	if !ok {
		return fmt.Errorf("no connection for chatID: %s", msg.ChatID)
	}

	var eventType string
	switch msg.MsgType {
	case "error":
		eventType = "error"
	default:
		eventType = "response"
	}

	data, _ := json.Marshal(map[string]interface{}{
		"content": msg.Content,
		"type":    msg.MsgType,
	})

	select {
	case conn.FlushCh <- fmt.Appendf(nil, "event: %s\ndata: %s\n\n", eventType, data):
	case <-ctx.Done():
		return ctx.Err()
	}

	return nil
}

// routeMessage routes an outbound message to the correct SSE connection
func (w *WebChannel) routeMessage(msg *bus.OutBoundMessage) {
	w.mu.RLock()
	conn, ok := w.connections[msg.ChatID]
	w.mu.RUnlock()

	if !ok {
		return
	}

	var eventType string
	switch msg.MsgType {
	case "error":
		eventType = "error"
	default:
		eventType = "response"
	}

	data, _ := json.Marshal(map[string]interface{}{
		"content": msg.Content,
		"type":    msg.MsgType,
		"chat_id": msg.ChatID,
	})

	payload := fmt.Appendf(nil, "event: %s\ndata: %s\n\n", eventType, data)

	select {
	case conn.FlushCh <- payload:
	case <-time.After(5 * time.Second):
		// connection is too slow, skip
	}
}

// RegisterConnection registers an SSE connection for a chatID
func (w *WebChannel) RegisterConnection(chatID string) *WebConnection {
	conn := &WebConnection{
		ChatID:  chatID,
		FlushCh: make(chan []byte, 100),
		DoneCh:  make(chan struct{}),
	}

	w.mu.Lock()
	w.connections[chatID] = conn
	w.mu.Unlock()

	return conn
}

// UnregisterConnection removes an SSE connection
func (w *WebChannel) UnregisterConnection(chatID string) {
	w.mu.Lock()
	if conn, ok := w.connections[chatID]; ok {
		close(conn.FlushCh)
		close(conn.DoneCh)
		delete(w.connections, chatID)
	}
	w.mu.Unlock()
}

// handleChat handles POST /api/chat with SSE streaming
func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.jsonResponse(w, http.StatusBadRequest, map[string]string{
			"error": "Invalid request body",
		})
		return
	}

	if req.Message == "" {
		s.jsonResponse(w, http.StatusBadRequest, map[string]string{
			"error": "message is required",
		})
		return
	}

	if req.AgentName == "" {
		req.AgentName = interfaces.DefaultAgentName
	}

	chatID := req.ChatID
	if chatID == "" {
		chatID = uuid.New().String()
	}

	// Register SSE connection
	webChan := getGlobalWebChannel()
	if webChan == nil {
		s.jsonResponse(w, http.StatusInternalServerError, map[string]string{
			"error": "WebChannel not initialized",
		})
		return
	}

	conn := webChan.RegisterConnection(chatID)
	defer webChan.UnregisterConnection(chatID)

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Expose-Headers", "Cache-Control")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	// Send initial event
	fmt.Fprintf(w, "event: connected\ndata: {\"chat_id\": \"%s\"}\n\n", chatID)
	flusher.Flush()

	// Publish message to bus
	inboundMsg := &bus.InBoundMessage{
		ID:        uuid.New().String(),
		SenderID:  "web",
		ChatID:    chatID,
		Content:   req.Message,
		TimeStamp: time.Now(),
	}

	if err := s.msgBus.PublishInBoundMessage(r.Context(), inboundMsg); err != nil {
		fmt.Fprintf(w, "event: error\ndata: {\"error\": \"%s\"}\n\n", err.Error())
		flusher.Flush()
		return
	}

	// Wait for response via SSE connection
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()

	for {
		select {
		case data, ok := <-conn.FlushCh:
			if !ok {
				return
			}
			w.Write(data)
			flusher.Flush()
		case <-conn.DoneCh:
			return
		case <-ctx.Done():
			fmt.Fprintf(w, "event: done\ndata: {\"chat_id\": \"%s\"}\n\n", chatID)
			flusher.Flush()
			return
		}
	}
}

// handleChatReset handles POST /api/chat/{chatId}/reset
func (s *Server) handleChatReset(w http.ResponseWriter, r *http.Request) {
	// TODO: implement session reset
	s.jsonResponse(w, http.StatusOK, map[string]string{
		"status": "ok",
	})
}

// globalWebChannel holds the singleton WebChannel
var (
	globalWebChan *WebChannel
	webChanOnce   sync.Once
)

// SetGlobalWebChannel sets the global WebChannel instance
func SetGlobalWebChannel(ch *WebChannel) {
	globalWebChan = ch
}

// getGlobalWebChannel returns the global WebChannel
func getGlobalWebChannel() *WebChannel {
	return globalWebChan
}

// RegisterWebChannel creates and registers the WebChannel with the channel manager
func RegisterWebChannel(msgBus *bus.MessageBus) *WebChannel {
	var once sync.Once
	var ch *WebChannel

	once.Do(func() {
		ch = NewWebChannel(msgBus)
		globalWebChan = ch
	})

	return ch
}
