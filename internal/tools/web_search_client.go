package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// SearchType 定义搜索类型
type SearchType string

const (
	SearchTypeWeb        SearchType = "web"
	SearchTypeWebSummary SearchType = "web_summary"
	SearchTypeImage      SearchType = "image"
)

// WebSearchRequest 搜索请求体
type WebSearchRequest struct {
	Query          string        `json:"Query"`
	SearchType     SearchType    `json:"SearchType"`
	Count          int           `json:"Count,omitempty"`
	Filter         *WebFilter    `json:"Filter,omitempty"`
	NeedSummary    bool          `json:"NeedSummary,omitempty"`
	TimeRange      string        `json:"TimeRange,omitempty"`
	QueryControl   *QueryControl `json:"QueryControl,omitempty"`
	ContentFormats string        `json:"ContentFormats,omitempty"`
	Industry       string        `json:"Industry,omitempty"`
}

// WebFilter web 搜索的过滤条件
type WebFilter struct {
	NeedContent   bool   `json:"NeedContent,omitempty"`
	NeedUrl       bool   `json:"NeedUrl,omitempty"`
	Sites         string `json:"Sites,omitempty"`
	BlockHosts    string `json:"BlockHosts,omitempty"`
	AuthInfoLevel int    `json:"AuthInfoLevel,omitempty"`
}

// ImageFilter image 搜索的过滤条件
type ImageFilter struct {
	ImageWidthMin  int      `json:"ImageWidthMin,omitempty"`
	ImageHeightMin int      `json:"ImageHeightMin,omitempty"`
	ImageWidthMax  int      `json:"ImageWidthMax,omitempty"`
	ImageHeightMax int      `json:"ImageHeightMax,omitempty"`
	ImageShapes    []string `json:"ImageShapes,omitempty"`
}

type QueryControl struct {
	QueryRewrite bool `json:"QueryRewrite,omitempty"`
}

// WebSearchResponse 搜索响应体
type WebSearchResponse struct {
	ResponseMetadata ResponseMetadata `json:"ResponseMetadata"`
	Result           SearchResult     `json:"Result"`
}

type ResponseMetadata struct {
	RequestId string `json:"RequestId"`
	Action    string `json:"Action"`
	Version   string `json:"Version"`
	Service   string `json:"Service"`
	Region    string `json:"Region"`
	Error     *Error `json:"Error,omitempty"`
}

type Error struct {
	CodeN   int    `json:"CodeN"`
	Code    string `json:"Code"`
	Message string `json:"Message"`
}

type SearchResult struct {
	ResultCount   int           `json:"ResultCount"`
	WebResults    []WebItem     `json:"WebResults,omitempty"`
	ImageResults  []ImageItem   `json:"ImageResults,omitempty"`
	Choices       []Choice      `json:"Choices,omitempty"`
	Usage         *Usage        `json:"Usage,omitempty"`
	SearchContext SearchContext `json:"SearchContext"`
	TimeCost      int           `json:"TimeCost"`
	LogId         string        `json:"LogId"`
	CardResults   []interface{} `json:"CardResults,omitempty"`
}

type WebItem struct {
	Id             string      `json:"Id"`
	SortId         int         `json:"SortId"`
	Title          string      `json:"Title"`
	SiteName       string      `json:"SiteName,omitempty"`
	Url            string      `json:"Url,omitempty"`
	Snippet        string      `json:"Snippet"`
	Summary        string      `json:"Summary,omitempty"`
	Content        string      `json:"Content,omitempty"`
	PublishTime    string      `json:"PublishTime,omitempty"`
	LogoUrl        string      `json:"LogoUrl,omitempty"`
	RankScore      float64     `json:"RankScore,omitempty"`
	AuthInfoDes    string      `json:"AuthInfoDes"`
	AuthInfoLevel  int         `json:"AuthInfoLevel"`
	ContentFormats string      `json:"ContentFormats,omitempty"`
	RuyiInfo       interface{} `json:"RuyiInfo,omitempty"`
}

type ImageItem struct {
	Id          string    `json:"Id"`
	SortId      int       `json:"SortId"`
	Title       string    `json:"Title,omitempty"`
	SiteName    string    `json:"SiteName,omitempty"`
	Url         string    `json:"Url,omitempty"`
	PublishTime string    `json:"PublishTime,omitempty"`
	Image       ImageInfo `json:"Image"`
}

type ImageInfo struct {
	Url       string `json:"Url"`
	Width     int    `json:"Width,omitempty"`
	Height    int    `json:"Height,omitempty"`
	Shape     string `json:"Shape"`
	BlurDes   string `json:"BlurDes,omitempty"`
	Category  string `json:"Category,omitempty"`
	Watermark string `json:"Watermark,omitempty"`
}

type Choice struct {
	Delta        *DeltaMessage `json:"Delta,omitempty"`
	Message      *DeltaMessage `json:"Message,omitempty"`
	FinishReason string        `json:"FinishReason"`
	Index        int           `json:"Index"`
}

type DeltaMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type Usage struct {
	PromptTokens       int `json:"PromptTokens"`
	CompletionTokens   int `json:"CompletionTokens"`
	TotalTokens        int `json:"TotalTokens"`
	SearchTimeCost     int `json:"SearchTimeCost"`
	FirstTokenTimeCost int `json:"FirstTokenTimeCost"`
	TotalTimeCost      int `json:"TotalTimeCost"`
}

type SearchContext struct {
	SearchType  string `json:"SearchType"`
	OriginQuery string `json:"OriginQuery"`
}

// WebSearchClient 负责请求 API
type WebSearchClient struct {
	APIKey string
	Client *http.Client
	APIURL string // 用于单元测试注入
}

func NewWebSearchClient(apiKey string) *WebSearchClient {
	return &WebSearchClient{
		APIKey: apiKey,
		APIURL: "https://open.feedcoopapi.com/search_api/web_search",
		Client: &http.Client{
			Timeout: 30 * time.Second, // 设置合理的超时时间
		},
	}
}

// Search 执行搜索请求
func (c *WebSearchClient) Search(ctx context.Context, req WebSearchRequest) (*WebSearchResponse, error) {
	reqBody, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("序列化搜索请求失败: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.APIURL, bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("创建搜索请求失败: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.APIKey)

	resp, err := c.Client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("发送搜索请求失败: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取搜索响应体失败: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("搜索API返回错误状态码(%d): %s", resp.StatusCode, string(bodyBytes))
	}

	var searchResp WebSearchResponse
	if err := json.Unmarshal(bodyBytes, &searchResp); err != nil {
		return nil, fmt.Errorf("解析搜索响应失败: %w", err)
	}

	// 检查 ResponseMetadata 中的错误
	if searchResp.ResponseMetadata.Error != nil {
		return nil, fmt.Errorf("搜索API内部错误(%s): %s", searchResp.ResponseMetadata.Error.Code, searchResp.ResponseMetadata.Error.Message)
	}

	return &searchResp, nil
}
