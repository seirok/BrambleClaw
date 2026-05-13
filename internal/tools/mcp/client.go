package mcp

import (
	"brambleclaw/internal/logger"
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
)

// Client MCP 客户端
type Client struct {
	transport Transport
	name      string

	nextID uint64
	// 当客户端发送 JSON-RPC 请求时，会生成一个唯一的请求 ID，并创建一个通道来接收响应
	pending map[uint64]chan *JSONRPCMessage
	mu      sync.Mutex

	ctx    context.Context
	cancel context.CancelFunc
}

// NewClient 创建一个新的 MCP 客户端
func NewClient(name string, transport Transport) *Client {
	ctx, cancel := context.WithCancel(context.Background())
	return &Client{
		transport: transport,
		name:      name,
		pending:   make(map[uint64]chan *JSONRPCMessage),
		ctx:       ctx,
		cancel:    cancel,
	}
}

// Start 启动客户端
func (c *Client) Start(ctx context.Context) error {
	if err := c.transport.Start(ctx); err != nil {
		return fmt.Errorf("启动传输层失败: %w", err)
	}

	go c.readLoop()

	// 发送 initialize 请求
	err := c.initialize(ctx)
	if err != nil {
		c.Close()
		return fmt.Errorf("MCP 初始化失败: %w", err)
	}

	return nil
}

func (c *Client) initialize(ctx context.Context) error {
	req := InitializeRequest{
		ProtocolVersion: "2024-11-05",
		Capabilities:    make(map[string]interface{}),
		ClientInfo: ClientInfo{
			Name:    "brambleclaw",
			Version: "1.0.0",
		},
	}

	resp, err := c.Call(ctx, "initialize", req)
	if err != nil {
		return err
	}

	var initResp InitializeResult
	if err := json.Unmarshal(resp.Result, &initResp); err != nil {
		return fmt.Errorf("解析 initialize 响应失败: %w", err)
	}

	// 发送 initialized 通知
	return c.Notify(ctx, "notifications/initialized", nil)
}

func (c *Client) readLoop() {
	for {
		msg, err := c.transport.Receive(c.ctx)
		if err != nil {
			logger.L().Error().Str("client", c.name).Err(err).Msg("MCP client failed to read message")
			return
		}

		var rpcMsg JSONRPCMessage
		if err := json.Unmarshal(msg, &rpcMsg); err != nil {
			logger.L().Error().Str("client", c.name).Err(err).Msg("MCP client failed to parse message")
			continue
		}

		// 处理响应
		if rpcMsg.ID != nil {
			idVal, ok := rpcMsg.ID.(float64)
			if ok {
				id := uint64(idVal)
				c.mu.Lock()
				ch, exists := c.pending[id]
				if exists {
					delete(c.pending, id)
				}
				c.mu.Unlock()

				if exists {
					ch <- &rpcMsg
				}
			}
		} else {
			// 处理通知或请求 (暂时忽略来自服务端的请求)
			logger.L().Debug().Str("client", c.name).Str("method", rpcMsg.Method).Msg("MCP client received notification")
		}
	}
}

// Call 发送 JSON-RPC 请求并等待响应 (同步阻塞）
func (c *Client) Call(ctx context.Context, method string, params interface{}) (*JSONRPCMessage, error) {
	id := atomic.AddUint64(&c.nextID, 1)

	var rawParams json.RawMessage
	if params != nil {
		b, err := json.Marshal(params)
		if err != nil {
			return nil, fmt.Errorf("序列化参数失败: %w", err)
		}
		rawParams = b
	}

	req := JSONRPCMessage{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  rawParams,
	}

	reqBytes, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("序列化请求失败: %w", err)
	}

	ch := make(chan *JSONRPCMessage, 1)
	c.mu.Lock()
	c.pending[id] = ch
	c.mu.Unlock()

	defer func() {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
	}()

	if err := c.transport.Send(ctx, reqBytes); err != nil {
		return nil, fmt.Errorf("发送请求失败: %w", err)
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-c.ctx.Done():
		return nil, fmt.Errorf("客户端已关闭")
	case resp := <-ch:
		if resp.Error != nil {
			return nil, fmt.Errorf("RPC 错误: [%d] %s", resp.Error.Code, resp.Error.Message)
		}
		return resp, nil
	}
}

// Notify 发送 JSON-RPC 通知 （异步）
func (c *Client) Notify(ctx context.Context, method string, params interface{}) error {
	var rawParams json.RawMessage
	if params != nil {
		b, err := json.Marshal(params)
		if err != nil {
			return fmt.Errorf("序列化参数失败: %w", err)
		}
		rawParams = b
	}

	req := JSONRPCMessage{
		JSONRPC: "2.0",
		Method:  method,
		Params:  rawParams,
	}

	reqBytes, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("序列化通知失败: %w", err)
	}

	if err := c.transport.Send(ctx, reqBytes); err != nil {
		return fmt.Errorf("发送通知失败: %w", err)
	}

	return nil
}

// ListTools 获取工具列表
func (c *Client) ListTools(ctx context.Context) ([]Tool, error) {
	resp, err := c.Call(ctx, "tools/list", nil)
	if err != nil {
		return nil, fmt.Errorf("调用 tools/list 失败: %w", err)
	}

	var result ListToolsResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return nil, fmt.Errorf("解析 tools/list 响应失败: %w", err)
	}

	return result.Tools, nil
}

// CallTool 调用工具
func (c *Client) CallTool(ctx context.Context, name string, args map[string]interface{}) (*CallToolResult, error) {
	req := CallToolRequest{
		Name:      name,
		Arguments: args,
	}

	resp, err := c.Call(ctx, "tools/call", req)
	if err != nil {
		return nil, fmt.Errorf("调用 tools/call 失败: %w", err)
	}

	var result CallToolResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return nil, fmt.Errorf("解析 tools/call 响应失败: %w", err)
	}

	return &result, nil
}

// Close 关闭客户端
func (c *Client) Close() error {
	c.cancel()
	return c.transport.Close()
}
