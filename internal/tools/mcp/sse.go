package mcp

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
)

// SSETransport 实现基于 Server-Sent Events 的传输层
type SSETransport struct {
	url     string
	headers map[string]string

	client  *http.Client
	postURL string

	msgChan chan []byte
	errChan chan error

	mu     sync.Mutex
	closed bool
	cancel context.CancelFunc
}

// NewSSETransport 创建一个新的 SSETransport
func NewSSETransport(url string, headers map[string]string) *SSETransport {
	return &SSETransport{
		url:     url,
		headers: headers,
		client:  &http.Client{},
		msgChan: make(chan []byte, 100),
		errChan: make(chan error, 1),
	}
}

// Start 启动传输层
func (t *SSETransport) Start(ctx context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.cancel != nil {
		return fmt.Errorf("传输层已启动")
	}

	ctx, cancel := context.WithCancel(ctx)
	t.cancel = cancel

	req, err := http.NewRequestWithContext(ctx, "GET", t.url, nil)
	if err != nil {
		return fmt.Errorf("创建 SSE 请求失败: %w", err)
	}

	req.Header.Set("Accept", "text/event-stream")
	for k, v := range t.headers {
		req.Header.Set(k, v)
	}

	resp, err := t.client.Do(req)
	if err != nil {
		return fmt.Errorf("发起 SSE 请求失败: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return fmt.Errorf("SSE 连接失败，状态码: %d", resp.StatusCode)
	}

	go t.readLoop(resp.Body)

	return nil
}

func (t *SSETransport) readLoop(body io.ReadCloser) {
	defer body.Close()
	reader := bufio.NewReader(body)

	var currentEvent string
	var currentData bytes.Buffer

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.errChan <- err
			return
		}

		line = strings.TrimSuffix(line, "\n")
		line = strings.TrimSuffix(line, "\r")

		if line == "" {
			// 空行表示一个事件的结束
			if currentEvent == "endpoint" {
				endpoint := strings.TrimSuffix(currentData.String(), "\n")

				t.mu.Lock()
				// 处理相对 URL
				base, err := url.Parse(t.url)
				if err == nil {
					ref, err := url.Parse(endpoint)
					if err == nil {
						t.postURL = base.ResolveReference(ref).String()
					} else {
						t.postURL = endpoint
					}
				} else {
					t.postURL = endpoint
				}
				t.mu.Unlock()
			} else if currentEvent == "message" {
				t.msgChan <- []byte(strings.TrimSuffix(currentData.String(), "\n"))
			}

			// 重置状态
			currentEvent = ""
			currentData.Reset()
			continue
		}

		if strings.HasPrefix(line, ":") {
			// 注释行，忽略
			continue
		}

		parts := strings.SplitN(line, ":", 2)
		if len(parts) == 2 {
			field := parts[0]
			value := parts[1]
			if strings.HasPrefix(value, " ") {
				value = value[1:]
			}

			switch field {
			case "event":
				currentEvent = value
			case "data":
				currentData.WriteString(value)
				currentData.WriteString("\n")
			}
		}
	}
}

// Send 发送 JSON-RPC 消息
func (t *SSETransport) Send(ctx context.Context, msg []byte) error {
	t.mu.Lock()
	postURL := t.postURL
	t.mu.Unlock()

	if postURL == "" {
		return fmt.Errorf("尚未收到 endpoint 事件，无法发送消息")
	}

	req, err := http.NewRequestWithContext(ctx, "POST", postURL, bytes.NewReader(msg))
	if err != nil {
		return fmt.Errorf("创建 POST 请求失败: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	for k, v := range t.headers {
		req.Header.Set(k, v)
	}

	resp, err := t.client.Do(req)
	if err != nil {
		return fmt.Errorf("发送 POST 请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("发送消息失败，状态码: %d", resp.StatusCode)
	}

	return nil
}

// Receive 接收 JSON-RPC 消息
func (t *SSETransport) Receive(ctx context.Context) ([]byte, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case msg := <-t.msgChan:
		return msg, nil
	case err := <-t.errChan:
		return nil, fmt.Errorf("SSE 连接断开: %w", err)
	}
}

// Close 关闭传输层
func (t *SSETransport) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed {
		return nil
	}
	t.closed = true

	if t.cancel != nil {
		t.cancel()
	}

	return nil
}
