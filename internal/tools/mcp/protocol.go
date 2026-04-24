package mcp

import "encoding/json"

// JSONRPCMessage 表示 JSON-RPC 消息
type JSONRPCMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id,omitempty"`     // 可以是 string 或 number
	Method  string          `json:"method,omitempty"` // Request 或 Notification 有
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"` // Response 成功时有
	Error   *JSONRPCError   `json:"error,omitempty"`  // Response 失败时有
}

// JSONRPCError 表示 JSON-RPC 错误
type JSONRPCError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// ClientInfo 客户端信息
type ClientInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// InitializeRequest params
type InitializeRequest struct {
	ProtocolVersion string                 `json:"protocolVersion"`
	Capabilities    map[string]interface{} `json:"capabilities"`
	ClientInfo      ClientInfo             `json:"clientInfo"`
}

// InitializeResult 结果
type InitializeResult struct {
	ProtocolVersion string                 `json:"protocolVersion"`
	Capabilities    map[string]interface{} `json:"capabilities"`
	ServerInfo      struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	} `json:"serverInfo"`
}

// Tool 工具结构
type Tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"inputSchema"` // JSON Schema
}

// ListToolsResult 返回
type ListToolsResult struct {
	Tools []Tool `json:"tools"`
}

// CallToolRequest 参数
type CallToolRequest struct {
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments,omitempty"`
}

// CallToolResult 结果
type CallToolResult struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	IsError bool `json:"isError,omitempty"`
}
