package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

func main() {
	// 配置信息
	apiKey := "9a44c2e7-40a3-4a89-a209-49c1cdcf6c6f"
	baseURL := "https://ark.cn-beijing.volces.com/api/coding/v3"
	model := "kimi-k2.5"

	// 构造请求体 (OpenAI 兼容格式)
	requestBody, _ := json.Marshal(map[string]interface{}{
		"model": model,
		"messages": []map[string]string{
			{"role": "user", "content": "你好，这是一条连接测试消息。"},
		},
	})

	// 创建请求
	req, err := http.NewRequest("POST", baseURL, bytes.NewBuffer(requestBody))
	if err != nil {
		fmt.Printf("创建请求失败: %v\n", err)
		return
	}

	// 设置 Header
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	// 发送请求
	client := &http.Client{Timeout: 30 * time.Second}
	fmt.Println("正在尝试连接...")
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("请求发生错误: %v\n", err)
		return
	}
	defer resp.Body.Close()

	// 读取并打印结果
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusOK {
		fmt.Println("✅ 连接成功！")
		fmt.Printf("响应内容: %s\n", string(body))
	} else {
		fmt.Printf("❌ 连接失败，状态码: %d\n", resp.StatusCode)
		fmt.Printf("错误详情: %s\n", string(body))
	}
}
