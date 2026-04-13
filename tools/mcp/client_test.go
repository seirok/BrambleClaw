package mcp

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"
)

type mockTransport struct {
	recvChan chan []byte
	sendChan chan []byte
	mu       sync.Mutex
	closed   bool
}

func newMockTransport() *mockTransport {
	return &mockTransport{
		recvChan: make(chan []byte, 10),
		sendChan: make(chan []byte, 10),
	}
}

func (m *mockTransport) Start(ctx context.Context) error {
	return nil
}

func (m *mockTransport) Send(ctx context.Context, msg []byte) error {
	m.sendChan <- msg
	return nil
}

func (m *mockTransport) Receive(ctx context.Context) ([]byte, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case msg := <-m.recvChan:
		return msg, nil
	}
}

func (m *mockTransport) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	return nil
}

func TestClient_Initialize(t *testing.T) {
	transport := newMockTransport()
	client := NewClient("test", transport)

	go func() {
		// Mock server behavior
		// 1. Receive initialize request
		reqBytes := <-transport.sendChan
		var req JSONRPCMessage
		json.Unmarshal(reqBytes, &req)

		// 2. Send initialize response
		resp := JSONRPCMessage{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  []byte(`{"protocolVersion":"2024-11-05","capabilities":{},"serverInfo":{"name":"mock","version":"1.0.0"}}`),
		}
		respBytes, _ := json.Marshal(resp)
		transport.recvChan <- respBytes

		// 3. Receive initialized notification
		<-transport.sendChan
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := client.Start(ctx)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer client.Close()
}

func TestClient_ListTools(t *testing.T) {
	transport := newMockTransport()
	client := NewClient("test", transport)

	// Bypass Start and manually set up for ListTools test
	client.ctx, client.cancel = context.WithCancel(context.Background())
	go client.readLoop()

	go func() {
		reqBytes := <-transport.sendChan
		var req JSONRPCMessage
		json.Unmarshal(reqBytes, &req)

		resp := JSONRPCMessage{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  []byte(`{"tools":[{"name":"test_tool","description":"A test tool","inputSchema":{"type":"object"}}]}`),
		}
		respBytes, _ := json.Marshal(resp)
		transport.recvChan <- respBytes
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	tools, err := client.ListTools(ctx)
	if err != nil {
		t.Fatalf("ListTools failed: %v", err)
	}

	if len(tools) != 1 {
		t.Fatalf("Expected 1 tool, got %d", len(tools))
	}
	if tools[0].Name != "test_tool" {
		t.Errorf("Expected tool name 'test_tool', got '%s'", tools[0].Name)
	}
}

func TestClient_CallTool(t *testing.T) {
	transport := newMockTransport()
	client := NewClient("test", transport)

	client.ctx, client.cancel = context.WithCancel(context.Background())
	go client.readLoop()

	go func() {
		reqBytes := <-transport.sendChan
		var req JSONRPCMessage
		json.Unmarshal(reqBytes, &req)

		resp := JSONRPCMessage{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  []byte(`{"content":[{"type":"text","text":"Success"}]}`),
		}
		respBytes, _ := json.Marshal(resp)
		transport.recvChan <- respBytes
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	res, err := client.CallTool(ctx, "test_tool", map[string]interface{}{"arg1": "val1"})
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}

	if res.IsError {
		t.Fatalf("Expected no error from tool call")
	}
	if len(res.Content) != 1 {
		t.Fatalf("Expected 1 content item, got %d", len(res.Content))
	}
	if res.Content[0].Text != "Success" {
		t.Errorf("Expected text 'Success', got '%s'", res.Content[0].Text)
	}
}
