package tools

import (
	"context"
	"fmt"
	"os"

	"github.com/bmatcuk/doublestar/v4"
)

// GlobTool 文件路径匹配工具
type GlobTool struct {
	*BaseTool
}

// NewGlobTool 创建 GlobTool
func NewGlobTool() *GlobTool {
	return &GlobTool{
		BaseTool: NewBaseTool(
			"glob",
			"文件路径匹配工具，支持 ** 递归匹配",
			nil,
			map[string]interface{}{
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
			},
		),
	}
}

// Execute 执行工具
func (t *GlobTool) Execute(ctx context.Context, argStr string) (interface{}, error) {
	t.LogStart()
	args, err := t.ParseArgs(argStr)
	if err != nil {
		return nil, err
	}

	pattern, ok := args["pattern"].(string)
	if !ok {
		return nil, fmt.Errorf("缺少必需参数: pattern")
	}

	basePath := "."
	if p, ok := args["path"].(string); ok && p != "" {
		basePath = t.ResolvePath(p)
	}

	// 使用 doublestar 进行匹配
	fs := os.DirFS(basePath)
	matches, err := doublestar.Glob(fs, pattern)
	if err != nil {
		return nil, fmt.Errorf("glob 匹配失败: %w", err)
	}

	return matches, nil
}
