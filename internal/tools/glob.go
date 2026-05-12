package tools

import (
	"brambleclaw/internal/logger"
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/bmatcuk/doublestar/v4"
)

// GlobTool 文件路径匹配工具
type GlobTool struct {
	name        string
	description string
}

// NewGlobTool 创建 GlobTool
func NewGlobTool() *GlobTool {
	return &GlobTool{
		name:        "glob",
		description: "文件路径匹配工具，支持 ** 递归匹配",
	}
}

// Name 返回工具名称
func (t *GlobTool) Name() string {
	return t.name
}

// Description 返回工具描述
func (t *GlobTool) Description() string {
	return t.description
}

// Parameters 返回参数定义
func (t *GlobTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"pattern": map[string]interface{}{
				"type":        "string",
				"description": "glob 模式，支持 *, ?, [], **",
			},
			"path": map[string]interface{}{
				"type":        "string",
				"description": "搜索起始路径（可选，默认当前目录）",
			},
		},
		"required": []string{"pattern"},
	}
}

// Execute 执行工具
func (t *GlobTool) Execute(ctx context.Context, argStr string) (interface{}, error) {
	logger.L().Info().Str("tool", t.name).Msg("tool start to execute")
	var args map[string]interface{}
	err := json.Unmarshal([]byte(argStr), &args)
	if err != nil {
		logger.L().Error().Err(err).Msg("解析失败")
		return nil, fmt.Errorf("解析失败: %w", err)
	}

	pattern, ok := args["pattern"].(string)
	if !ok {
		return nil, fmt.Errorf("缺少必需参数: pattern")
	}

	basePath := "."
	if p, ok := args["path"].(string); ok && p != "" {
		basePath = p
	}

	// 使用 doublestar 进行匹配
	fs := os.DirFS(basePath)
	matches, err := doublestar.Glob(fs, pattern)
	if err != nil {
		logger.L().Error().Err(err).Str("pattern", pattern).Msg("glob 匹配失败")
		return nil, fmt.Errorf("glob 匹配失败: %w", err)
	}

	// 返回匹配结果（已经是相对于 basePath 的路径）
	result := make([]string, 0, len(matches))
	for _, m := range matches {
		result = append(result, m)
	}

	return result, nil
}
