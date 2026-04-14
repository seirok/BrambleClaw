package tools

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestUrlParseTool_Execute(t *testing.T) {
	// 启动一个测试服务器
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/html":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Write([]byte(`<html>
				<head>
					<title>Test HTML</title>
					<style>body { color: red; }</style>
					<script>alert('test');</script>
				</head>
				<body>
					<h1>Hello World</h1>
					<p>This is a <b>test</b> paragraph.</p>
				</body>
			</html>`))
		case "/json":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"key": "value", "list": [1, 2, 3]}`))
		case "/raw":
			w.Header().Set("Content-Type", "text/plain")
			w.Write([]byte(`Plain text content`))
		case "/large":
			// 生成大量数据以测试截断
			w.Header().Set("Content-Type", "text/plain")
			data := strings.Repeat("A", 100000+10)
			w.Write([]byte(data))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer ts.Close()

	tool := NewUrlParseTool()

	// 由于测试服务器运行在 localhost，默认的 UrlParseTool 会拦截本地 IP 访问以防 SSRF。
	// 为了测试，我们需要替换 client.Transport 为不拦截本地 IP 的 Transport。
	tool.client = ts.Client()

	tests := []struct {
		name          string
		url           string
		wantErr       bool
		checkContains []string
		extractor     string
	}{
		{
			name:          "Test HTML Parsing",
			url:           ts.URL + "/html",
			wantErr:       false,
			checkContains: []string{"Hello World", "This is a test paragraph."},
			extractor:     "html",
		},
		{
			name:          "Test JSON Parsing",
			url:           ts.URL + "/json",
			wantErr:       false,
			checkContains: []string{`"key": "value"`, `1,`},
			extractor:     "json",
		},
		{
			name:          "Test Raw Parsing",
			url:           ts.URL + "/raw",
			wantErr:       false,
			checkContains: []string{"Plain text content"},
			extractor:     "raw",
		},
		{
			name:    "Test Invalid URL",
			url:     "invalid-url",
			wantErr: true,
		},
		{
			name:    "Test Not Supported Scheme",
			url:     "ftp://example.com",
			wantErr: true,
		},
		{
			name:          "Test Large Content Truncation",
			url:           ts.URL + "/large",
			wantErr:       false,
			checkContains: []string{"...[内容过长已截断]..."},
			extractor:     "raw",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := UrlParseRequest{Url: tt.url}
			argsBytes, _ := json.Marshal(req)

			res, err := tool.Execute(context.Background(), string(argsBytes))
			if (err != nil) != tt.wantErr {
				t.Errorf("Execute() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				resStr, ok := res.(string)
				if !ok {
					t.Errorf("Execute() returned non-string result")
					return
				}

				var parseResult UrlParseResult
				if err := json.Unmarshal([]byte(resStr), &parseResult); err != nil {
					t.Errorf("Failed to parse result JSON: %v", err)
					return
				}

				if parseResult.ExtractorType != tt.extractor {
					t.Errorf("Expected extractor %s, got %s", tt.extractor, parseResult.ExtractorType)
				}

				for _, s := range tt.checkContains {
					if !strings.Contains(parseResult.Content, s) {
						t.Errorf("Expected content to contain %q, but it didn't. Content: %q", s, parseResult.Content)
					}
				}

				// html 解析时不应该包含被过滤的内容
				if tt.extractor == "html" {
					if strings.Contains(parseResult.Content, "alert('test')") {
						t.Errorf("HTML text should not contain script content")
					}
					if strings.Contains(parseResult.Content, "body { color: red; }") {
						t.Errorf("HTML text should not contain style content")
					}
				}
			}
		})
	}
}

func TestUrlParseTool_SSRF_Protection(t *testing.T) {
	// 使用默认的 tool，它包含了拦截本地 IP 的逻辑
	tool := NewUrlParseTool()

	// 尝试访问本地测试服务器，应该被拦截
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("Hello"))
	}))
	defer ts.Close()

	req := UrlParseRequest{Url: ts.URL}
	argsBytes, _ := json.Marshal(req)

	_, err := tool.Execute(context.Background(), string(argsBytes))
	if err == nil {
		t.Errorf("Expected SSRF protection to block local IP access, but it succeeded")
	} else if !strings.Contains(err.Error(), "禁止访问私有或本地IP") && !strings.Contains(err.Error(), "无可用安全IP可连接") {
		t.Errorf("Expected error to contain SSRF block message, got: %v", err)
	}
}

func TestIsSafeIP(t *testing.T) {
	tests := []struct {
		ip   string
		safe bool
	}{
		{"127.0.0.1", false},
		{"10.0.0.1", false},
		{"192.168.1.1", false},
		{"172.16.0.1", false},
		{"169.254.169.254", false},
		{"8.8.8.8", true},
		{"114.114.114.114", true},
		{"::1", false},
		{"2001:4860:4860::8888", true},
	}

	for _, tt := range tests {
		ip := net.ParseIP(tt.ip)
		if isSafeIP(ip) != tt.safe {
			t.Errorf("isSafeIP(%s) = %v, want %v", tt.ip, !tt.safe, tt.safe)
		}
	}
}

// TestRealUrlParseTool_Execute 测试真实URL的内容提取
func TestRealUrlParseTool_Execute(t *testing.T) {
	tool := NewUrlParseTool()

	// 测试真实的微博登录页面
	t.Run("WeiboLoginPage", func(t *testing.T) {
		targetURL := "https://weibo.com/newlogin"

		req := UrlParseRequest{Url: targetURL}
		argsBytes, _ := json.Marshal(req)

		result, err := tool.Execute(context.Background(), string(argsBytes))
		if err != nil {
			t.Fatalf("执行 UrlParse 失败: %v", err)
		}

		// 验证结果
		resultStr, ok := result.(string)
		if !ok {
			t.Fatalf("Expected string result, got %T", result)
		}

		// 解析结果
		var parseResult UrlParseResult
		if err := json.Unmarshal([]byte(resultStr), &parseResult); err != nil {
			t.Fatalf("解析结果JSON失败: %v", err)
		}

		// 打印提取结果
		t.Logf("\n=== 网页内容提取结果 ===")
		t.Logf("URL: %s", parseResult.Url)
		t.Logf("状态码: %d", parseResult.StatusCode)
		t.Logf("提取器类型: %s", parseResult.ExtractorType)
		t.Logf("内容长度: %d 字符", parseResult.ContentLength)
		t.Logf("=== 提取的内容 ===")
		t.Log(parseResult.Content)
		t.Logf("=== 内容提取结束 ===")

		// 验证结果包含预期内容
		if parseResult.StatusCode != 200 {
			t.Errorf("Expected status code 200, got %d", parseResult.StatusCode)
		}

		if parseResult.ExtractorType != "html" {
			t.Errorf("Expected extractor type 'html', got %s", parseResult.ExtractorType)
		}

		// 验证内容不为空
		if strings.TrimSpace(parseResult.Content) == "" {
			t.Errorf("Expected non-empty content")
		}

		// 验证包含微博相关关键词
		expectedKeywords := []string{"微博", "登录", "Weibo"}
		foundKeywords := 0
		for _, keyword := range expectedKeywords {
			if strings.Contains(parseResult.Content, keyword) {
				foundKeywords++
			}
		}

		// 至少找到一个关键词
		if foundKeywords == 0 {
			t.Logf("警告: 未找到预期关键词，可能页面结构已变化")
		} else {
			t.Logf("找到 %d 个预期关键词: %v", foundKeywords, expectedKeywords)
		}

		// 验证HTML标签被正确过滤
		if strings.Contains(parseResult.Content, "<script>") || strings.Contains(parseResult.Content, "</script>") {
			t.Errorf("HTML标签应该被过滤，但发现script标签")
		}

		if strings.Contains(parseResult.Content, "<style>") || strings.Contains(parseResult.Content, "</style>") {
			t.Errorf("HTML标签应该被过滤，但发现style标签")
		}
	})
}
