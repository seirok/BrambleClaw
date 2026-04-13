package mcp

import (
	"context"
)

// Transport 定义了 MCP 客户端与服务端之间的传输层接口
type Transport interface {
	// Start 启动传输层
	Start(ctx context.Context) error

	// Send 发送 JSON-RPC 消息
	Send(ctx context.Context, msg []byte) error

	// Receive 接收 JSON-RPC 消息
	Receive(ctx context.Context) ([]byte, error)

	// Close 关闭传输层
	Close() error
}
