package config

import (
	"brambleclaw/internal/logger"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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

// ValidateLLMConfig 验证 LLM 配置
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
