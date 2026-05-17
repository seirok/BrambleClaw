package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// WebSearchTool 联网搜索工具
type WebSearchTool struct {
	*BaseTool
	client *WebSearchClient
}

// NewWebSearchTool 创建 WebSearch 工具
func NewWebSearchTool(apiKey string) *WebSearchTool {
	return &WebSearchTool{
		BaseTool: NewBaseTool(
			"web_search",
			"使用搜索引擎获取互联网上的最新信息。支持搜索网页、网页带总结和图片。",
			nil,
			map[string]any{
				"type": "object",
				"properties": map[string]any{
					"Query": map[string]any{
						"type":        "string",
						"description": "用户搜索query，1~100个字符，不支持多词搜索",
					},
					"SearchType": map[string]any{
						"type":        "string",
						"enum":        []string{"web", "web_summary", "image"},
						"description": "搜索类型",
					},
					"Count": map[string]any{
						"type":        "integer",
						"description": "返回条数，最多50条（image最多5条），默认10条",
					},
					"NeedSummary": map[string]any{
						"type":        "boolean",
						"description": "是否需要精准摘要。当 SearchType 为 web_summary 时，必须为 true",
					},
					"TimeRange": map[string]any{
						"type":        "string",
						"enum":        []string{"OneDay", "OneWeek", "OneMonth", "OneYear"},
						"description": "指定搜索的发文时间范围",
					},
				},
				"required": []string{"Query", "SearchType"},
			},
		),
		client: NewWebSearchClient(apiKey),
	}
}

// Execute 执行搜索工具
func (w *WebSearchTool) Execute(ctx context.Context, args string) (any, error) {
	w.LogStart()
	var req WebSearchRequest
	if err := json.Unmarshal([]byte(args), &req); err != nil {
		return nil, fmt.Errorf("解析 WebSearch 参数失败: %w", err)
	}

	resp, err := w.client.Search(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("执行搜索请求失败: %w", err)
	}

	var result strings.Builder
	fmt.Fprintf(&result, "搜索结果总数: %d\n", resp.Result.ResultCount)

	if req.SearchType == SearchTypeWeb || req.SearchType == SearchTypeWebSummary {
		if len(resp.Result.WebResults) > 0 {
			result.WriteString("--- 网页结果 ---\n")
			for i, item := range resp.Result.WebResults {
				fmt.Fprintf(&result, "[%d] 标题: %s\n", i+1, item.Title)
				if item.SiteName != "" {
					fmt.Fprintf(&result, "    来源: %s\n", item.SiteName)
				}
				if item.Url != "" {
					fmt.Fprintf(&result, "    链接: %s\n", item.Url)
				}
				if item.Summary != "" {
					fmt.Fprintf(&result, "    摘要: %s\n", item.Summary)
				} else if item.Snippet != "" {
					fmt.Fprintf(&result, "    片段: %s\n", item.Snippet)
				}
				if item.Content != "" {
					fmt.Fprintf(&result, "    正文: %s\n", item.Content)
				}
				result.WriteString("\n")
			}
		}
		if req.SearchType == SearchTypeWebSummary && len(resp.Result.Choices) > 0 {
			result.WriteString("--- 模型总结 ---\n")
			for _, choice := range resp.Result.Choices {
				if choice.Message != nil && choice.Message.Content != "" {
					result.WriteString(choice.Message.Content)
					result.WriteString("\n")
				}
			}
		}
	} else if req.SearchType == SearchTypeImage {
		if len(resp.Result.ImageResults) > 0 {
			result.WriteString("--- 图片结果 ---\n")
			for i, item := range resp.Result.ImageResults {
				fmt.Fprintf(&result, "[%d] 标题: %s\n", i+1, item.Title)
				fmt.Fprintf(&result, "    图片链接: %s\n", item.Image.Url)
				fmt.Fprintf(&result, "    尺寸: %dx%d\n", item.Image.Width, item.Image.Height)
				result.WriteString("\n")
			}
		}
	}

	if result.Len() == 0 {
		return "没有找到相关的结果。", nil
	}
	return strings.TrimSpace(result.String()), nil
}
