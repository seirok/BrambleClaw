package agent

import (
	"brambleclaw/internal/config"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// LLMClient LLM客户端: 对 LLMProcessor 的实现
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

// Complete 完成聊天
func (c *LLMClient) Model() string {
	modelName := c.config.Model
	// TODO: check
	return modelName
}

func (c *LLMClient) Chat(msgs ChatCompletionRequest) (*LLMResponse, error) {
	//
	data, err := json.Marshal(msgs)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal ChatMsg: %v", err)
	}
	httpReq, err := http.NewRequest("POST", c.config.BaseURL, bytes.NewBuffer(data)) // a
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
