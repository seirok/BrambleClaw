package tools

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"brambleclaw/config"
)

func TestWebSearchTool_Execute_Web(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("Expected method POST, got %s", r.Method)
		}
		if r.Header.Get("Authorization") != "Bearer test_key" {
			t.Errorf("Expected Bearer test_key, got %s", r.Header.Get("Authorization"))
		}

		resp := WebSearchResponse{
			ResponseMetadata: ResponseMetadata{
				RequestId: "test_req_id",
			},
			Result: SearchResult{
				ResultCount: 1,
				WebResults: []WebItem{
					{
						Title:    "Test Title",
						SiteName: "Test Site",
						Url:      "http://test.com",
						Snippet:  "Test Snippet",
					},
				},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	tool := NewWebSearchTool("test_key")
	tool.client.APIURL = ts.URL // 重写为测试服务器地址

	args := `{"Query": "test query", "SearchType": "web"}`
	res, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	resStr, ok := res.(string)
	if !ok {
		t.Fatalf("Expected string result, got %T", res)
	}

	if !strings.Contains(resStr, "Test Title") {
		t.Errorf("Result does not contain expected title, got: %s", resStr)
	}
	if !strings.Contains(resStr, "Test Site") {
		t.Errorf("Result does not contain expected site name, got: %s", resStr)
	}
}

func TestWebSearchTool_Parameters(t *testing.T) {
	tool := NewWebSearchTool("test_key")
	params := tool.Parameters()

	if params["type"] != "object" {
		t.Errorf("Expected type object, got %v", params["type"])
	}

	props, ok := params["properties"].(map[string]interface{})
	if !ok {
		t.Fatalf("Expected properties to be map[string]interface{}")
	}

	if _, ok := props["Query"]; !ok {
		t.Errorf("Expected property Query")
	}

	if _, ok := props["SearchType"]; !ok {
		t.Errorf("Expected property SearchType")
	}

	reqs, ok := params["required"].([]string)
	if !ok {
		t.Fatalf("Expected required to be []string")
	}

	foundQuery := false
	foundSearchType := false
	for _, req := range reqs {
		if req == "Query" {
			foundQuery = true
		}
		if req == "SearchType" {
			foundSearchType = true
		}
	}

	if !foundQuery || !foundSearchType {
		t.Errorf("Expected required to contain Query and SearchType")
	}
}

func TestRealWebSearchTool_Execute(t *testing.T) {

	// 加载配置文件（使用相对项目根目录的路径）
	cfg, err := config.Load("../config/config.json")
	if err != nil {
		t.Fatalf("加载配置文件失败: %v", err)
	}

	// 获取 WebSearch API Key
	apiKey := cfg.Tools.WebSearch.APIKey
	if apiKey == "" {
		t.Skip("WebSearch API Key 未配置")
	}

	// 创建 WebSearch 工具
	tool := NewWebSearchTool(apiKey)

	// 测试网页搜索
	t.Run("WebSearch", func(t *testing.T) {
		args := `{"Query": "2024年奥运会举办城市", "SearchType": "web", "Count": 5}`
		result, err := tool.Execute(context.Background(), args)
		if err != nil {
			t.Fatalf("执行 WebSearch 失败: %v", err)
		}

		// 验证结果
		resultStr, ok := result.(string)
		if !ok {
			t.Fatalf("Expected string result, got %T", result)
		}

		// 打印搜索结果
		t.Logf("\n=== 网页搜索结果 ===")
		t.Log(resultStr)
		t.Logf("=== 搜索结果结束 ===")

		// 验证结果包含预期内容
		if !strings.Contains(resultStr, "搜索结果总数") {
			t.Errorf("结果中应包含搜索结果总数")
		}
		if !strings.Contains(resultStr, "--- 网页结果 ---") {
			t.Errorf("结果中应包含网页结果部分")
		}
	})

	// 测试带总结的网页搜索
	t.Run("WebSearchWithSummary", func(t *testing.T) {
		args := `{"Query": "2024年最新科技趋势", "SearchType": "web_summary", "Count": 3, "NeedSummary": true}`
		result, err := tool.Execute(context.Background(), args)
		if err != nil {
			t.Fatalf("执行 WebSearch 失败: %v", err)
		}

		// 验证结果
		resultStr, ok := result.(string)
		if !ok {
			t.Fatalf("Expected string result, got %T", result)
		}

		// 打印搜索结果
		t.Logf("\n=== 带总结的网页搜索结果 ===")
		t.Log(resultStr)
		t.Logf("=== 搜索结果结束 ===")

		// 验证结果包含预期内容
		if !strings.Contains(resultStr, "搜索结果总数") {
			t.Errorf("结果中应包含搜索结果总数")
		}
		if !strings.Contains(resultStr, "--- 网页结果 ---") {
			t.Errorf("结果中应包含网页结果部分")
		}
		if !strings.Contains(resultStr, "--- 模型总结 ---") {
			t.Errorf("结果中应包含模型总结部分")
		}
	})

	// 测试图片搜索
	t.Run("ImageSearch", func(t *testing.T) {
		args := `{"Query": "cat", "SearchType": "image", "Count": 3}`
		result, err := tool.Execute(context.Background(), args)
		if err != nil {
			t.Fatalf("执行 ImageSearch 失败: %v", err)
		}

		// 验证结果
		resultStr, ok := result.(string)
		if !ok {
			t.Fatalf("Expected string result, got %T", result)
		}

		// 打印搜索结果
		t.Logf("\n=== 图片搜索结果 ===")
		t.Log(resultStr)
		t.Logf("=== 搜索结果结束 ===")

		// 验证结果包含预期内容
		if !strings.Contains(resultStr, "搜索结果总数") {
			t.Errorf("结果中应包含搜索结果总数")
		}
		if !strings.Contains(resultStr, "--- 图片结果 ---") {
			t.Errorf("结果中应包含图片结果部分")
		}
	})
}
