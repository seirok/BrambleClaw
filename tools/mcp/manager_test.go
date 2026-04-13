package mcp

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestMCPToolWrapper_Parameters(t *testing.T) {
	wrapper := NewMCPToolWrapper("test_tool", Tool{
		Name:        "test",
		Description: "A test tool",
		InputSchema: json.RawMessage(`{"type": "object", "properties": {"param1": {"type": "string"}}}`),
	}, nil)

	params := wrapper.Parameters()
	if params == nil {
		t.Fatalf("Parameters() returned nil")
	}

	typ, ok := params["type"].(string)
	if !ok || typ != "object" {
		t.Errorf("Expected type 'object', got %v", params["type"])
	}
}

func TestMCPToolWrapper_Execute(t *testing.T) {
	transport := newMockTransport()
	client := NewClient("test", transport)

	client.ctx, client.cancel = context.WithCancel(context.Background())
	go client.readLoop()

	wrapper := NewMCPToolWrapper("test_tool", Tool{
		Name:        "test",
		Description: "A test tool",
		InputSchema: json.RawMessage(`{}`),
	}, client)

	go func() {
		reqBytes := <-transport.sendChan
		var req JSONRPCMessage
		json.Unmarshal(reqBytes, &req)

		resp := JSONRPCMessage{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  []byte(`{"content":[{"type":"text","text":"Executed successfully"}],"isError":false}`),
		}
		respBytes, _ := json.Marshal(resp)
		transport.recvChan <- respBytes
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	result, err := wrapper.Execute(ctx, `{"param1": "value1"}`)
	if err != nil {
		t.Fatalf("Execute() failed: %v", err)
	}

	resStr, ok := result.(string)
	if !ok {
		t.Fatalf("Expected result to be string, got %T", result)
	}

	if resStr != "Executed successfully" {
		t.Errorf("Expected 'Executed successfully', got '%s'", resStr)
	}
}

func TestMCPToolWrapper_ExecuteError(t *testing.T) {
	transport := newMockTransport()
	client := NewClient("test", transport)

	client.ctx, client.cancel = context.WithCancel(context.Background())
	go client.readLoop()

	wrapper := NewMCPToolWrapper("test_tool", Tool{
		Name:        "test",
		Description: "A test tool",
		InputSchema: json.RawMessage(`{}`),
	}, client)

	go func() {
		reqBytes := <-transport.sendChan
		var req JSONRPCMessage
		json.Unmarshal(reqBytes, &req)

		resp := JSONRPCMessage{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  []byte(`{"content":[{"type":"text","text":"Tool failed"}],"isError":true}`),
		}
		respBytes, _ := json.Marshal(resp)
		transport.recvChan <- respBytes
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err := wrapper.Execute(ctx, `{}`)
	if err == nil {
		t.Fatalf("Expected error, got nil")
	}

	expectedErr := "工具执行返回错误(test_tool): Tool failed"
	if err.Error() != expectedErr {
		t.Errorf("Expected error '%s', got '%s'", expectedErr, err.Error())
	}
}
