package mcp

import (
	"brambleclaw/internal/config"
	"brambleclaw/internal/hook"
	"brambleclaw/internal/logger"
	"brambleclaw/internal/tools"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
)

// Manager 管理多个 MCP 客户端连接
type Manager struct {
	config  config.MCPConfig
	clients map[string]*Client
	mu      sync.Mutex
}

// NewManager 创建新的 MCP Manager
func NewManager(cfg config.MCPConfig) *Manager {
	return &Manager{
		config:  cfg,
		clients: make(map[string]*Client),
	}
}

// Start 启动并初始化所有配置的 MCP 服务器
func (m *Manager) Start(ctx context.Context, registry *tools.ToolRegistry) error {
	if !m.config.Enabled {
		return nil
	}

	for name, srvCfg := range m.config.Servers {
		if !srvCfg.Enabled {
			continue
		}

		client, err := m.createClient(name, srvCfg)
		if err != nil {
			logger.L().Error().Str("client", name).Err(err).Msg("创建 MCP 客户端失败")
			continue
		}

		if err := client.Start(ctx); err != nil {
			logger.L().Error().Str("client", name).Err(err).Msg("启动 MCP 客户端失败")
			client.Close()
			continue
		}

		m.mu.Lock()
		m.clients[name] = client
		m.mu.Unlock()

		// 注册工具
		mcpTools, err := client.ListTools(ctx)
		if err != nil {
			logger.L().Error().Str("client", name).Err(err).Msg("获取 MCP 客户端工具列表失败")
			continue
		}

		for _, t := range mcpTools {
			toolName := fmt.Sprintf("%s_%s", name, t.Name)
			wrapper := NewMCPToolWrapper(toolName, t, client)
			registry.Register(ctx, toolName, wrapper)
			logger.L().Debug().Str("tool", toolName).Msg("注册 MCP 工具")
		}
	}

	return nil
}

// createClient 根据配置创建客户端
func (m *Manager) createClient(name string, cfg config.MCPServerConfig) (*Client, error) {
	var transport Transport

	switch strings.ToLower(cfg.Type) {
	case "stdio":
		if cfg.Command == "" {
			return nil, fmt.Errorf("stdio 类型的服务器命令不能为空")
		}

		// 加载环境变量文件
		var env []string
		if cfg.EnvFile != "" {
			content, err := os.ReadFile(cfg.EnvFile)
			if err != nil {
				logger.L().Error().Str("env_file", cfg.EnvFile).Err(err).Msg("读取环境变量文件失败")
			} else {
				lines := strings.Split(string(content), "\n")
				for _, line := range lines {
					line = strings.TrimSpace(line)
					if line != "" && !strings.HasPrefix(line, "#") {
						env = append(env, line)
					}
				}
			}
		}

		transport = NewStdioTransport(cfg.Command, cfg.Args, env)

	case "sse":
		if cfg.URL == "" {
			return nil, fmt.Errorf("sse 类型的服务器 URL 不能为空")
		}
		transport = NewSSETransport(cfg.URL, cfg.Headers)

	default:
		return nil, fmt.Errorf("不支持的 MCP 服务器类型: %s", cfg.Type)
	}

	return NewClient(name, transport), nil
}

// Stop 关闭所有客户端连接
func (m *Manager) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for name, client := range m.clients {
		if err := client.Close(); err != nil {
			logger.L().Error().Str("client", name).Err(err).Msg("关闭 MCP 客户端失败")
		} else {
			logger.L().Debug().Str("client", name).Msg("关闭 MCP 客户端成功")
		}
	}
	m.clients = make(map[string]*Client)
}

// MCPToolWrapper 将 MCP 工具包装为 Agent 的 tools.Tool 接口，和本地工具一起供给Agent选择
// 当 MCP 管理器启动时，会执行以下步骤：
//
// 1. 创建 MCP 客户端 ：根据配置创建对应类型（stdio/sse）的客户端
// 2. 启动客户端 ：建立与 MCP 服务器的连接
// 3. 初始化连接 ：发送 initialize 请求
// 4. 获取工具列表 ：调用 tools/list 获取 MCP 服务器提供的工具
// 5. 注册工具 ：为每个工具创建 MCPToolWrapper 并注册到 Agent 的工具注册表
type MCPToolWrapper struct {
	name        string
	description string
	inputSchema json.RawMessage
	client      *Client
	original    string // 原始工具名称
}

func NewMCPToolWrapper(name string, mcpTool Tool, client *Client) *MCPToolWrapper {
	return &MCPToolWrapper{
		name:        name,
		description: mcpTool.Description,
		inputSchema: mcpTool.InputSchema,
		client:      client,
		original:    mcpTool.Name,
	}
}

func (w *MCPToolWrapper) Name() string {
	return w.name
}

func (w *MCPToolWrapper) Description() string {
	return w.description
}

func (w *MCPToolWrapper) Parameters() map[string]interface{} {
	var schema map[string]interface{}
	if err := json.Unmarshal(w.inputSchema, &schema); err != nil {
		logger.L().Error().Err(err).Msg("解析工具参数 schema 失败")
		return nil
	}
	return schema
}

func (w *MCPToolWrapper) Execute(ctx context.Context, args string) (interface{}, error) {
	var parsedArgs map[string]interface{}
	if args != "" {
		if err := json.Unmarshal([]byte(args), &parsedArgs); err != nil {
			return nil, fmt.Errorf("解析工具参数失败(%s): %w", w.name, err)
		}
	}

	// 触发 MCP 工具执行前钩子
	hookData := map[string]interface{}{
		"tool_name":     w.name,
		"original_name": w.original,
		"args":          args,
	}
	if processedData, err := hook.Emit(ctx, "hook.point.mcp.tool.pre-execute", hookData); err != nil {
		return nil, fmt.Errorf("MCP 工具执行前钩子拒绝执行: %w", err)
	} else if processedData != nil {
		// 钩子可能修改了参数
		if newArgs, ok := processedData.(string); ok && newArgs != args {
			args = newArgs
			// 重新解析参数
			parsedArgs = nil
			if args != "" {
				if err := json.Unmarshal([]byte(args), &parsedArgs); err != nil {
					return nil, fmt.Errorf("解析修改后的工具参数失败(%s): %w", w.name, err)
				}
			}
		}
	}

	res, err := w.client.CallTool(ctx, w.original, parsedArgs)
	if err != nil {
		return nil, fmt.Errorf("执行 MCP 工具失败(%s): %w", w.name, err)
	}

	if res.IsError {
		// 返回格式化的错误结果
		var errMsg strings.Builder
		for _, c := range res.Content {
			if c.Type == "text" {
				errMsg.WriteString(c.Text)
				errMsg.WriteString("\n")
			}
		}
		return nil, fmt.Errorf("工具执行返回错误(%s): %s", w.name, strings.TrimSpace(errMsg.String()))
	}

	// 聚合所有返回的文本内容
	var result strings.Builder
	for _, c := range res.Content {
		if c.Type == "text" {
			result.WriteString(c.Text)
			result.WriteString("\n")
		}
	}

	return strings.TrimSpace(result.String()), nil
}
