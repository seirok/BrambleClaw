package tools

import (
	"brambleclaw/internal/logger"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	// SandboxAddr 沙箱服务的内部地址
	SandboxAddr = "http://127.0.0.1:8080"
)

// CodeSandboxTool 代码执行沙箱工具
type CodeSandboxTool struct {
	client *http.Client
}

// NewCodeSandboxTool 创建沙箱工具实例
func NewCodeSandboxTool() *CodeSandboxTool {
	return &CodeSandboxTool{
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (t *CodeSandboxTool) Name() string {
	return "code_sandbox"
}

func (t *CodeSandboxTool) Description() string {
	return "在安全的隔离环境中执行编程代码。支持多种语言（如 Python, C++, Go, Node.js）。当你需要进行数学计算、数据处理、算法验证或测试代码片段时，请使用此工具。"
}

func (t *CodeSandboxTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"language": map[string]interface{}{
				"type":        "string",
				"enum":        []string{"python", "cpp", "go", "nodejs"},
				"description": "编程语言类型",
			},
			"code": map[string]interface{}{
				"type":        "string",
				"description": "要执行的完整源代码",
			},
		},
		"required": []string{"language", "code"},
	}
}

// Execute 执行代码沙箱
func (t *CodeSandboxTool) Execute(ctx context.Context, args string) (interface{}, error) {
	logger.L().Debug().Str("tool", t.Name()).Msg("Starting CodeSandbox tool execution")

	// 1. 解析参数
	var req struct {
		Language string `json:"language"`
		Code     string `json:"code"`
	}
	if err := json.Unmarshal([]byte(args), &req); err != nil {
		logger.L().Error().Err(err).Msg("Failed to parse CodeSandbox parameters")
		return nil, fmt.Errorf("解析参数失败: %w", err)
	}

	// 2. 准备发送给容器的请求体
	sandboxReq := map[string]interface{}{
		"language": req.Language,
		"code":     req.Code,
		"timeout":  10, // 内部执行超时
	}
	jsonData, _ := json.Marshal(sandboxReq)

	// 3. 调用沙箱 API
	httpReq, err := http.NewRequestWithContext(ctx, "POST", SandboxAddr+"/run_code", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := t.client.Do(httpReq)
	if err != nil {
		logger.L().Error().Err(err).Msg("Failed to connect to sandbox service")
		return nil, fmt.Errorf("沙箱服务暂不可用: %w", err)
	}
	defer resp.Body.Close()

	// 4. 读取并解析返回结果
	body, _ := io.ReadAll(resp.Body)
	var res struct {
		Status    string `json:"status"`
		Message   string `json:"message"`
		RunResult struct {
			ReturnCode int     `json:"return_code"`
			Stdout     string  `json:"stdout"`
			Stderr     string  `json:"stderr"`
			Time       float64 `json:"execution_time"`
		} `json:"run_result"`
	}

	if err := json.Unmarshal(body, &res); err != nil {
		return nil, fmt.Errorf("解析沙箱返回数据失败: %w", err)
	}

	// 5. 格式化输出给 LLM
	var output strings.Builder
	output.WriteString(fmt.Sprintf("状态: %s\n", res.Status))

	if res.Status != "Success" {
		output.WriteString(fmt.Sprintf("错误信息: %s\n", res.Message))
	}

	if res.RunResult.Stdout != "" {
		output.WriteString("--- 标准输出 (Stdout) ---\n")
		output.WriteString(res.RunResult.Stdout)
		output.WriteString("\n")
	}

	if res.RunResult.Stderr != "" {
		output.WriteString("--- 标准错误 (Stderr) ---\n")
		output.WriteString(res.RunResult.Stderr)
		output.WriteString("\n")
	}

	output.WriteString(fmt.Sprintf("退出码: %d | 执行耗时: %.4fs", res.RunResult.ReturnCode, res.RunResult.Time))

	return output.String(), nil
}
