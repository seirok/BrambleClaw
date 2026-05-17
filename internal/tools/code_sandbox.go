package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const SandboxAddr = "http://127.0.0.1:8080"

// CodeSandboxTool 代码执行沙箱工具
type CodeSandboxTool struct {
	*BaseTool
	client *http.Client
}

// NewCodeSandboxTool 创建沙箱工具实例
func NewCodeSandboxTool() *CodeSandboxTool {
	return &CodeSandboxTool{
		BaseTool: NewBaseTool(
			"code_sandbox",
			"在安全的隔离环境中执行编程代码。支持多种语言（如 Python, C++, Go, Node.js）。",
			nil,
			map[string]any{
				"type": "object",
				"properties": map[string]any{
					"language": map[string]any{
						"type":        "string",
						"enum":        []string{"python", "cpp", "go", "nodejs"},
						"description": "编程语言类型",
					},
					"code": map[string]any{
						"type":        "string",
						"description": "要执行的完整源代码",
					},
				},
				"required": []string{"language", "code"},
			},
		),
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

// Execute 执行代码沙箱
func (t *CodeSandboxTool) Execute(ctx context.Context, args string) (any, error) {
	t.LogStart()
	var req struct {
		Language string `json:"language"`
		Code     string `json:"code"`
	}
	if err := json.Unmarshal([]byte(args), &req); err != nil {
		return nil, fmt.Errorf("解析参数失败: %w", err)
	}

	sandboxReq := map[string]any{
		"language": req.Language,
		"code":     req.Code,
		"timeout":  10,
	}
	jsonData, _ := json.Marshal(sandboxReq)

	httpReq, err := http.NewRequestWithContext(ctx, "POST", SandboxAddr+"/run_code", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := t.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("沙箱服务暂不可用: %w", err)
	}
	defer resp.Body.Close()

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

	var output strings.Builder
	fmt.Fprintf(&output, "状态: %s\n", res.Status)
	if res.Status != "Success" {
		fmt.Fprintf(&output, "错误信息: %s\n", res.Message)
	}
	if res.RunResult.Stdout != "" {
		output.WriteString("--- 标准输出 ---\n")
		output.WriteString(res.RunResult.Stdout)
		output.WriteString("\n")
	}
	if res.RunResult.Stderr != "" {
		output.WriteString("--- 标准错误 ---\n")
		output.WriteString(res.RunResult.Stderr)
		output.WriteString("\n")
	}
	fmt.Fprintf(&output, "退出码: %d | 耗时: %.4fs", res.RunResult.ReturnCode, res.RunResult.Time)

	return output.String(), nil
}
