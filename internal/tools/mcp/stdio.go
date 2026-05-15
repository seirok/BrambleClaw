package mcp

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
)

// StdioTransport 实现基于标准输入输出的传输层
type StdioTransport struct {
	command string
	args    []string
	env     []string

	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser
	reader *bufio.Reader

	msgChan chan []byte
	errChan chan error

	mu     sync.Mutex
	closed bool
}

// NewStdioTransport 创建一个新的 StdioTransport
func NewStdioTransport(command string, args []string, env []string) *StdioTransport {
	return &StdioTransport{
		command: command,
		args:    args,
		env:     env,
		msgChan: make(chan []byte, 100),
		errChan: make(chan error, 1),
	}
}

// Start 启动子进程
func (t *StdioTransport) Start(ctx context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.cmd != nil {
		return fmt.Errorf("传输层已启动")
	}

	cmd := exec.CommandContext(ctx, t.command, t.args...)

	// 设置环境变量
	if len(t.env) > 0 {
		cmd.Env = append(os.Environ(), t.env...)
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("获取标准输入管道失败(%s): %w", t.command, err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("获取标准输出管道失败(%s): %w", t.command, err)
	}

	// 重定向标准错误以便于调试
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("启动进程失败(%s): %w", t.command, err)
	}

	t.cmd = cmd
	t.stdin = stdin
	t.stdout = stdout
	t.reader = bufio.NewReader(stdout)

	go t.readLoop()

	return nil
}

func (t *StdioTransport) readLoop() {
	for {
		line, err := t.reader.ReadBytes('\n')

		// 1. 处理读取到的数据
		if len(line) > 0 {
			trimmed := strings.TrimSpace(string(line))
			// 健壮性检查：只有看起来像 JSON 对象的行才送入通道
			if strings.HasPrefix(trimmed, "{") && strings.HasSuffix(trimmed, "}") {
				// 发送副本，避免底层数组竞争
				msg := make([]byte, len(line))
				copy(msg, line)
				t.msgChan <- msg
			} else if trimmed != "" {
				// 将非协议输出打印到日志，方便调试（例如那行 "Running on stdio"）
				// 这里可以使用你的 logger 记录
				fmt.Printf("[MCP Server Debug] %s\n", trimmed)
			}
		}

		// 2. 处理错误/退出
		if err != nil {
			t.mu.Lock()
			isClosed := t.closed
			t.mu.Unlock()

			if !isClosed {
				t.errChan <- err
			}
			return
		}
	}
}

// Send 发送 JSON-RPC 消息
func (t *StdioTransport) Send(ctx context.Context, msg []byte) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed {
		return fmt.Errorf("传输层已关闭")
	}

	// 确保消息以换行符结束
	msgStr := string(msg)
	if !strings.HasSuffix(msgStr, "\n") {
		msgStr += "\n"
	}

	_, err := t.stdin.Write([]byte(msgStr))
	if err != nil {
		return fmt.Errorf("写入标准输入失败: %w", err)
	}

	return nil
}

// Receive 接收 JSON-RPC 消息
func (t *StdioTransport) Receive(ctx context.Context) ([]byte, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case msg := <-t.msgChan:
		return msg, nil
	case err := <-t.errChan:
		return nil, fmt.Errorf("读取标准输出失败: %w", err)
	}
}

// Close 关闭传输层
func (t *StdioTransport) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed {
		return nil
	}
	t.closed = true

	var err error
	if t.stdin != nil {
		err = t.stdin.Close()
	}

	// 可选：等待进程退出，或杀死它
	if t.cmd != nil && t.cmd.Process != nil {
		_ = t.cmd.Process.Kill()
		_ = t.cmd.Wait()
	}

	return err
}
