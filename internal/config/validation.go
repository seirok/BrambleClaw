package config

import (
	"brambleclaw/internal/config/structs"
	"brambleclaw/internal/logger"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// LLMTestRequest LLM 测试请求
type LLMTestRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
}

// Message 消息结构
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// LLMTestResponse LLM 测试响应
type LLMTestResponse struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	Model   string `json:"model"`
	Choices []struct {
		Index   int `json:"index"`
		Message struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

// LLMValidationResult LLM 验证结果
type LLMValidationResult struct {
	Success   bool          `json:"success"`
	Message   string        `json:"message"`
	Latency   time.Duration `json:"latency"`
	TokenUsed int           `json:"token_used"`
}

type Validator interface {
	Validate() error
}

func (c *Config) Validate() error {
	if err := validateWorkspace(c.Workspace); err != nil {
		return err
	}

	if err := validateLogConfig(structs.LogConfig(c.Log)); err != nil {
		return err
	}

	if err := validateSessionConfig(structs.SessionConfig(c.Session)); err != nil {
		return err
	}

	if err := validateLLMConfig(structs.LLMConfig(c.LLMConfig)); err != nil {
		return err
	}

	if err := validateSandboxConfig(structs.SandboxConfig(c.Sandbox)); err != nil {
		return err
	}

	return nil
}

func validateWorkspace(workspace string) error {
	if !filepath.IsAbs(workspace) {
		return fmt.Errorf("工作空间路径必须是绝对路径: %s", workspace)
	}

	info, err := os.Stat(workspace)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("工作空间目录不存在: %s", workspace)
		}
		return err
	}

	if !info.IsDir() {
		return fmt.Errorf("工作空间路径必须是一个目录: %s", workspace)
	}

	return nil
}

func validateLogConfig(log structs.LogConfig) error {
	if log.Path == "" {
		return fmt.Errorf("日志路径不能为空")
	}

	if log.Level == "" {
		return fmt.Errorf("日志级别不能为空")
	}

	return nil
}

func validateSessionConfig(session structs.SessionConfig) error {
	if session.StorageFormat != "jsonl" {
		return fmt.Errorf("不支持的存储格式: %s", session.StorageFormat)
	}

	if session.MaxHistory < 0 {
		return fmt.Errorf("最大历史消息数不能小于0")
	}

	return nil
}

// ValidateLLMConfig 验证 LLM 配置（详细测试）
func ValidateLLMConfig(ctx context.Context, config LLMConfig) (*LLMValidationResult, error) {
	logger.L().Debug().
		Str("base_url", config.BaseURL).
		Str("model", config.Model).
		Msg("开始验证 LLM 配置")

	// 构建测试请求
	testReq := LLMTestRequest{
		Model: config.Model,
		Messages: []Message{
			{
				Role:    "user",
				Content: "Hello, this is a test message. Please respond with 'Test successful'.",
			},
		},
	}

	reqBody, err := json.Marshal(testReq)
	if err != nil {
		return nil, fmt.Errorf("序列化测试请求失败: %w", err)
	}

	// 创建 HTTP 请求
	req, err := http.NewRequestWithContext(ctx, "POST", config.BaseURL, bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, fmt.Errorf("创建 HTTP 请求失败: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+config.APIKey)

	// 发送请求并计时
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	start := time.Now()
	resp, err := client.Do(req)
	latency := time.Since(start)

	if err != nil {
		return &LLMValidationResult{
			Success: false,
			Message: fmt.Sprintf("请求失败: %v", err),
			Latency: latency,
		}, nil
	}
	defer resp.Body.Close()

	// 读取响应体
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return &LLMValidationResult{
			Success: false,
			Message: fmt.Sprintf("读取响应失败: %v", err),
			Latency: latency,
		}, nil
	}

	// 检查状态码
	if resp.StatusCode != http.StatusOK {
		return &LLMValidationResult{
			Success: false,
			Message: fmt.Sprintf("HTTP 错误: %d, 响应: %s", resp.StatusCode, string(body)),
			Latency: latency,
		}, nil
	}

	// 解析响应
	var llmResp LLMTestResponse
	if err := json.Unmarshal(body, &llmResp); err != nil {
		return &LLMValidationResult{
			Success: false,
			Message: fmt.Sprintf("解析响应失败: %v", err),
			Latency: latency,
		}, nil
	}

	// 验证响应内容
	if len(llmResp.Choices) == 0 {
		return &LLMValidationResult{
			Success: false,
			Message: "响应中没有 choices",
			Latency: latency,
		}, nil
	}

	tokenUsed := llmResp.Usage.TotalTokens
	logger.L().Debug().
		Str("model", llmResp.Model).
		Int("tokens", tokenUsed).
		Dur("latency", latency).
		Msg("LLM 验证成功")

	return &LLMValidationResult{
		Success:   true,
		Message:   fmt.Sprintf("验证成功，模型: %s", llmResp.Model),
		Latency:   latency,
		TokenUsed: tokenUsed,
	}, nil
}

// validateLLMConfig LLM 配置的基本验证
func validateLLMConfig(llm structs.LLMConfig) error {
	// LLM API Key 可以为空，因为可能通过其他方式提供（如环境变量）
	if llm.BaseURL == "" {
		return fmt.Errorf("LLM Base URL 不能为空")
	}

	if llm.Model == "" {
		return fmt.Errorf("LLM 模型名称不能为空")
	}

	return nil
}

func validateSandboxConfig(sandbox structs.SandboxConfig) error {
	if sandbox.Workspace == "" {
		return fmt.Errorf("沙箱工作目录不能为空")
	}

	if sandbox.Execution.Timeout <= 0 {
		return fmt.Errorf("执行超时时间必须大于0")
	}

	if sandbox.Execution.MaxOutputSize <= 0 {
		return fmt.Errorf("最大输出大小必须大于0")
	}

	return nil
}
