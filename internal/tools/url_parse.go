package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/net/html"
)

const maxContentLength = 5 * 1024 * 1024

// UrlParseTool 网页内容获取和解析工具
type UrlParseTool struct {
	*BaseTool
	client *http.Client
}

// NewUrlParseTool 创建 UrlParse 工具
func NewUrlParseTool() *UrlParseTool {
	dialer := &net.Dialer{
		Timeout:   5 * time.Second,
		KeepAlive: 30 * time.Second,
	}

	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, err
			}
			ips, err := net.LookupIP(host)
			if err != nil {
				return nil, fmt.Errorf("解析域名IP失败(%s): %w", host, err)
			}

			var lastErr error
			for _, ip := range ips {
				if !isSafeIP(ip) {
					continue
				}

				safeAddr := net.JoinHostPort(ip.String(), port)
				conn, err := dialer.DialContext(ctx, network, safeAddr)
				if err == nil {
					return conn, nil
				}
				lastErr = err
			}

			if lastErr != nil {
				return nil, lastErr
			}
			return nil, fmt.Errorf("无可用安全IP可连接(%s)", host)
		},
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   15 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return fmt.Errorf("重定向次数过多")
			}
			return nil
		},
	}

	return &UrlParseTool{
		BaseTool: NewBaseTool(
			"url_parse",
			"获取并提取特定网页的内容。支持HTML文本提取、JSON格式化输出等。仅允许HTTP/HTTPS协议，且禁止访问本地或私有网络。",
			nil,
			map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"Url": map[string]interface{}{
						"type":        "string",
						"description": "要访问的网页URL，必须以http://或https://开头",
					},
				},
				"required": []string{"Url"},
			},
		),
		client: client,
	}
}

// UrlParseRequest 请求参数结构
type UrlParseRequest struct {
	Url string `json:"Url"`
}

// UrlParseResult 格式化返回结果
type UrlParseResult struct {
	Url           string `json:"url"`
	StatusCode    int    `json:"status_code"`
	ExtractorType string `json:"extractor_type"`
	ContentLength int    `json:"content_length"`
	Content       string `json:"content"`
}

// Execute 执行网页内容获取和解析
func (u *UrlParseTool) Execute(ctx context.Context, args string) (interface{}, error) {
	u.LogStart()
	var req UrlParseRequest
	if err := json.Unmarshal([]byte(args), &req); err != nil {
		return nil, fmt.Errorf("解析UrlParse参数失败: %w", err)
	}

	result, err := u.doFetchAndParse(ctx, req.Url)
	if err != nil {
		return nil, err
	}

	resBytes, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("序列化UrlParseResult失败: %w", err)
	}
	return string(resBytes), nil
}

func (u *UrlParseTool) doFetchAndParse(ctx context.Context, targetUrl string) (*UrlParseResult, error) {
	parsedUrl, err := url.Parse(targetUrl)
	if err != nil {
		return nil, fmt.Errorf("解析URL失败: %w", err)
	}

	if parsedUrl.Scheme != "http" && parsedUrl.Scheme != "https" {
		return nil, fmt.Errorf("不支持的URL协议: %s", parsedUrl.Scheme)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "GET", targetUrl, nil)
	if err != nil {
		return nil, fmt.Errorf("创建HTTP请求失败: %w", err)
	}

	httpReq.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	httpReq.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8")
	httpReq.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")

	resp, err := u.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("执行HTTP请求失败: %w", err)
	}
	defer resp.Body.Close()

	limitReader := io.LimitReader(resp.Body, maxContentLength)
	bodyBytes, err := io.ReadAll(limitReader)
	if err != nil {
		return nil, fmt.Errorf("读取HTTP响应内容失败: %w", err)
	}

	contentStr := string(bodyBytes)
	contentType := resp.Header.Get("Content-Type")
	extractorType := "raw"

	if strings.Contains(strings.ToLower(contentType), "text/html") {
		extractorType = "html"
		contentStr = extractTextFromHTML(contentStr)
	} else if strings.Contains(strings.ToLower(contentType), "application/json") {
		extractorType = "json"
		contentStr = formatJSON(contentStr)
	}

	maxOutputChars := 100000
	if len(contentStr) > maxOutputChars {
		contentStr = contentStr[:maxOutputChars] + "\n\n...[内容过长已截断]..."
	}

	return &UrlParseResult{
		Url:           targetUrl,
		StatusCode:    resp.StatusCode,
		ExtractorType: extractorType,
		ContentLength: len(contentStr),
		Content: contentStr,
	}, nil
}

func isSafeIP(ip net.IP) bool {
	return !(ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsMulticast() || ip.IsUnspecified())
}

func extractTextFromHTML(htmlStr string) string {
	doc, err := html.Parse(strings.NewReader(htmlStr))
	if err != nil {
		return htmlStr
	}

	var sb strings.Builder
	var extract func(*html.Node)
	extract = func(n *html.Node) {
		if n.Type == html.ElementNode {
			switch n.Data {
			case "script", "style", "noscript", "iframe", "svg", "img":
				return
			}
		}
		if n.Type == html.TextNode {
			text := strings.TrimSpace(n.Data)
			if text != "" {
				sb.WriteString(text)
				sb.WriteString(" ")
			}
		}
		if n.Type == html.ElementNode {
			switch n.Data {
			case "p", "div", "br", "h1", "h2", "h3", "h4", "h5", "h6", "li", "tr", "article", "section":
				sb.WriteString("\n")
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			extract(c)
		}
		if n.Type == html.ElementNode {
			switch n.Data {
			case "p", "div", "h1", "h2", "h3", "h4", "h5", "h6", "li", "tr", "article", "section":
				sb.WriteString("\n")
			}
		}
	}
	extract(doc)

	lines := strings.Split(sb.String(), "\n")
	var cleanedLines []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			cleanedLines = append(cleanedLines, trimmed)
		}
	}
	return strings.Join(cleanedLines, "\n")
}

func formatJSON(jsonStr string) string {
	var obj interface{}
	if err := json.Unmarshal([]byte(jsonStr), &obj); err != nil {
		return jsonStr
	}
	bytes, err := json.MarshalIndent(obj, "", "  ")
	if err != nil {
		return jsonStr
	}
	return string(bytes)
}
