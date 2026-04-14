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
	"miniGoClaw/logger"
)

const maxContentLength = 5 * 1024 * 1024 // 5MB 最大读取限制

// UrlParseTool 网页内容获取和解析工具
type UrlParseTool struct {
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
				return nil, fmt.Errorf("解析地址端口失败(%s): %w", addr, err)
			}
			ips, err := net.LookupIP(host)
			if err != nil {
				return nil, fmt.Errorf("解析域名IP失败(%s): %w", host, err)
			}

			var lastErr error
			for _, ip := range ips {
				if !isSafeIP(ip) {
					logger.L().Warn().Str("ip", ip.String()).Msg("拦截到访问私有或本地IP地址")
					lastErr = fmt.Errorf("禁止访问私有或本地IP(%s)", ip.String())
					continue
				}

				safeAddr := net.JoinHostPort(ip.String(), port)
				conn, err := dialer.DialContext(ctx, network, safeAddr)
				if err == nil {
					return conn, nil
				}
				lastErr = fmt.Errorf("连接安全IP失败(%s): %w", safeAddr, err)
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
				return fmt.Errorf("重定向次数过多(%d)", len(via))
			}
			return nil
		},
	}

	return &UrlParseTool{
		client: client,
	}
}

// isSafeIP 判断IP是否为安全的公网IP，防止SSRF
func isSafeIP(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsMulticast() || ip.IsUnspecified() {
		return false
	}
	return true
}

func (u *UrlParseTool) Name() string {
	return "url_parse"
}

func (u *UrlParseTool) Description() string {
	return "获取并提取特定网页的内容。支持HTML文本提取、JSON格式化输出等。仅允许HTTP/HTTPS协议，且禁止访问本地或私有网络。"
}

func (u *UrlParseTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"Url": map[string]interface{}{
				"type":        "string",
				"description": "要访问的网页URL，必须以http://或https://开头",
			},
		},
		"required": []string{"Url"},
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
	logger.L().Debug().Str("tool", u.Name()).Msg("开始执行 UrlParse 工具")

	var req UrlParseRequest
	if err := json.Unmarshal([]byte(args), &req); err != nil {
		err = fmt.Errorf("解析UrlParse参数失败(%s): %w", args, err)
		logger.L().Error().Err(err).Msg("工具参数解析错误")
		return nil, err
	}

	result, err := u.doFetchAndParse(ctx, req.Url)
	if err != nil {
		logger.L().Error().Err(err).Str("url", req.Url).Msg("获取或解析网页内容失败")
		return nil, err
	}

	resBytes, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		err = fmt.Errorf("序列化UrlParseResult失败(%s): %w", req.Url, err)
		logger.L().Error().Err(err).Msg("序列化结果错误")
		return nil, err
	}

	logger.L().Debug().Str("url", req.Url).Int("content_length", result.ContentLength).Msg("UrlParse 执行成功")
	return string(resBytes), nil
}

func (u *UrlParseTool) doFetchAndParse(ctx context.Context, targetUrl string) (*UrlParseResult, error) {
	parsedUrl, err := url.Parse(targetUrl)
	if err != nil {
		return nil, fmt.Errorf("解析URL失败(%s): %w", targetUrl, err)
	}

	if parsedUrl.Scheme != "http" && parsedUrl.Scheme != "https" {
		return nil, fmt.Errorf("不支持的URL协议(%s): 仅支持http/https", parsedUrl.Scheme)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "GET", targetUrl, nil)
	if err != nil {
		return nil, fmt.Errorf("创建HTTP请求失败(%s): %w", targetUrl, err)
	}

	// 模拟常见浏览器 User-Agent，防止简单的反爬拦截
	httpReq.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	httpReq.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8")
	httpReq.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")

	logger.L().Debug().Str("url", targetUrl).Msg("开始发起HTTP请求")
	resp, err := u.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("执行HTTP请求失败(%s): %w", targetUrl, err)
	}
	defer resp.Body.Close()

	logger.L().Debug().Int("status_code", resp.StatusCode).Msg("收到HTTP响应")

	limitReader := io.LimitReader(resp.Body, maxContentLength)
	bodyBytes, err := io.ReadAll(limitReader)
	if err != nil {
		return nil, fmt.Errorf("读取HTTP响应内容失败(%s): %w", targetUrl, err)
	}

	contentStr := string(bodyBytes)
	contentType := resp.Header.Get("Content-Type")
	extractorType := "raw"

	logger.L().Debug().Str("content_type", contentType).Msg("开始处理响应内容")

	if strings.Contains(strings.ToLower(contentType), "text/html") {
		extractorType = "html"
		contentStr = extractTextFromHTML(contentStr)
	} else if strings.Contains(strings.ToLower(contentType), "application/json") {
		extractorType = "json"
		contentStr = formatJSON(contentStr)
	}

	// 限制内容字符数，防止过大结果导致大模型超载
	maxOutputChars := 100000 // 限制10万字符
	if len(contentStr) > maxOutputChars {
		contentStr = contentStr[:maxOutputChars] + "\n\n...[内容过长已截断]..."
	}

	return &UrlParseResult{
		Url:           targetUrl,
		StatusCode:    resp.StatusCode,
		ExtractorType: extractorType,
		ContentLength: len(contentStr),
		Content:       contentStr,
	}, nil
}

// extractTextFromHTML 从HTML字符串中提取纯文本内容
func extractTextFromHTML(htmlStr string) string {
	doc, err := html.Parse(strings.NewReader(htmlStr))
	if err != nil {
		return htmlStr // 解析失败则降级返回原文
	}

	var sb strings.Builder
	var extract func(*html.Node)
	extract = func(n *html.Node) {
		if n.Type == html.ElementNode {
			// 过滤不需要的标签
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

		// 块级元素换行处理，提升可读性
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

	// 清理多余空行和空格
	text := sb.String()
	lines := strings.Split(text, "\n")
	var cleanedLines []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			cleanedLines = append(cleanedLines, trimmed)
		}
	}

	return strings.Join(cleanedLines, "\n")
}

// formatJSON 格式化JSON字符串
func formatJSON(jsonStr string) string {
	var obj interface{}
	if err := json.Unmarshal([]byte(jsonStr), &obj); err != nil {
		return jsonStr // 解析失败降级返回原文
	}
	bytes, err := json.MarshalIndent(obj, "", "  ")
	if err != nil {
		return jsonStr
	}
	return string(bytes)
}
