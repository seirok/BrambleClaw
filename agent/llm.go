package agent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"miniGoClaw/config"
	"net/http"
)

// LLMConfig LLM配置

type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}
type LLMResponse struct {
	ID      string `json:"id"`
	Choices []struct {
		Message struct {
			Role      string     `json:"role"`
			Content   string     `json:"content"`
			ToolCalls []ToolCall `json:"tool_calls"`
		} `json:"message"`
	} `json:"choices"`
}

// LLMClient LLM客户端
type LLMClient struct {
	config config.LLMConfig
	client *http.Client
}

// NewLLMClient 创建LLM客户端
func NewLLMClient(config config.LLMConfig) *LLMClient {
	return &LLMClient{
		config: config,
		client: &http.Client{},
	}
}

// ChatCompletionRequest 聊天请求（需严格对齐官方API doc）
type ChatCompletionRequest struct {
	Model    string                   `json:"model"`
	Messages []ChatMsg                `json:"messages"`
	Tools    []map[string]interface{} `json:"tools,omitempty"`
}

// ChatCompletionResponse 聊天完成响应

// Complete 完成聊天
func (c *LLMClient) Chat(msgs ChatCompletionRequest) (*LLMResponse, error) {
	//
	data, err := json.Marshal(msgs)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal ChatMsg: %v", err)
	}
	httpReq, err := http.NewRequest("POST", c.config.BaseURL, bytes.NewBuffer(data)) // ?
	if err != nil {
		return nil, fmt.Errorf("failed to create http request: %v", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.config.APIKey)
	httpResp, err := c.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send http request: %v", err)
	}
	if httpResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(httpResp.Body)
		return nil, fmt.Errorf("status=%d, body=%s", httpResp.StatusCode, string(body))
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {

		}
	}(httpResp.Body)

	//
	var chatResp LLMResponse
	if err = json.NewDecoder(httpResp.Body).Decode(&chatResp); err != nil {
		return nil, fmt.Errorf("failed to decode json response: %v", err)
	}

	return &chatResp, nil
}
