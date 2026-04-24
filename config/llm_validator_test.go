package config

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestValidateLLMConfig_Success(t *testing.T) {
	// 创建模拟服务器
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 验证请求头
		authHeader := r.Header.Get("Authorization")
		if authHeader != "Bearer test-api-key" {
			t.Errorf("期望的 Authorization 头: Bearer test-api-key, 实际: %s", authHeader)
		}

		// 返回成功响应
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{
			"id": "test-id",
			"object": "chat.completion",
			"created": 1234567890,
			"model": "gpt-3.5-turbo",
			"choices": [{
				"index": 0,
				"message": {
					"role": "assistant",
					"content": "Test successful"
				},
				"finish_reason": "stop"
			}],
			"usage": {
				"prompt_tokens": 10,
				"completion_tokens": 5,
				"total_tokens": 15
			}
		}`))
	}))
	defer server.Close()

	// 测试配置
	config := LLMConfig{
		APIKey:  "test-api-key",
		BaseURL: server.URL,
		Model:   "gpt-3.5-turbo",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := ValidateLLMConfig(ctx, config)
	if err != nil {
		t.Fatalf("验证失败: %v", err)
	}

	if !result.Success {
		t.Errorf("期望验证成功，但失败了: %s", result.Message)
	}

	if result.TokenUsed != 15 {
		t.Errorf("期望 Token 使用量为 15，实际: %d", result.TokenUsed)
	}

	if result.Latency <= 0 {
		t.Errorf("期望延迟大于 0，实际: %v", result.Latency)
	}
}

func TestValidateLLMConfig_InvalidAPIKey(t *testing.T) {
	// 创建返回 401 的模拟服务器
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error": "Invalid API key"}`))
	}))
	defer server.Close()

	config := LLMConfig{
		APIKey:  "invalid-key",
		BaseURL: server.URL,
		Model:   "gpt-3.5-turbo",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := ValidateLLMConfig(ctx, config)
	if err != nil {
		t.Fatalf("验证返回错误: %v", err)
	}

	if result.Success {
		t.Error("期望验证失败，但成功了")
	}

	if result.Message == "" {
		t.Error("失败时应该返回错误消息")
	}
}

func TestValidateLLMConfig_Timeout(t *testing.T) {
	// 创建延迟响应的模拟服务器
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	config := LLMConfig{
		APIKey:  "test-key",
		BaseURL: server.URL,
		Model:   "gpt-3.5-turbo",
	}

	// 使用很短的超时
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	result, err := ValidateLLMConfig(ctx, config)
	if err != nil {
		t.Fatalf("验证返回错误: %v", err)
	}

	if result.Success {
		t.Error("期望超时验证失败")
	}
}
