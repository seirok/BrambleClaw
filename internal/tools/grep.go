package tools

import (
	"bufio"
	"brambleclaw/internal/logger"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/bmatcuk/doublestar/v4"
)

// GrepTool 文件内容搜索工具
type GrepTool struct {
	name        string
	description string
}

// GrepMatch 单个匹配结果
type GrepMatch struct {
	File    string   `json:"file"`
	Line    int      `json:"line,omitempty"`
	Content string   `json:"content"`
	Before  []string `json:"before,omitempty"`
	After   []string `json:"after,omitempty"`
}

// NewGrepTool 创建 GrepTool
func NewGrepTool() *GrepTool {
	return &GrepTool{
		name:        "grep",
		description: "文件内容搜索工具，支持字符串匹配和正则表达式",
	}
}

// Name 返回工具名称
func (t *GrepTool) Name() string {
	return t.name
}

// Description 返回工具描述
func (t *GrepTool) Description() string {
	return t.description
}

// Parameters 返回参数定义
func (t *GrepTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"pattern": map[string]interface{}{
				"type":        "string",
				"description": "搜索模式（字符串或正则表达式）",
			},
			"path": map[string]interface{}{
				"type":        "string",
				"description": "搜索路径（可选，默认当前目录）",
			},
			"is_regex": map[string]interface{}{
				"type":        "boolean",
				"description": "是否使用正则表达式（可选，默认 false）",
			},
			"recursive": map[string]interface{}{
				"type":        "boolean",
				"description": "是否递归搜索子目录（可选，默认 true）",
			},
			"show_line_num": map[string]interface{}{
				"type":        "boolean",
				"description": "是否显示行号（可选，默认 true）",
			},
			"before_context": map[string]interface{}{
				"type":        "number",
				"description": "匹配前显示的行数（可选，默认 0）",
			},
			"after_context": map[string]interface{}{
				"type":        "number",
				"description": "匹配后显示的行数（可选，默认 0）",
			},
			"include": map[string]interface{}{
				"type":        "string",
				"description": "包含的文件模式（可选）",
			},
			"exclude": map[string]interface{}{
				"type":        "string",
				"description": "排除的文件模式（可选）",
			},
		},
		"required": []string{"pattern"},
	}
}

// Execute 执行工具
func (t *GrepTool) Execute(ctx context.Context, argStr string) (interface{}, error) {
	var args struct {
		Pattern       string  `json:"pattern"`
		Path          string  `json:"path"`
		IsRegex       bool    `json:"is_regex"`
		Recursive     bool    `json:"recursive"`
		ShowLineNum   bool    `json:"show_line_num"`
		BeforeContext int     `json:"before_context"`
		AfterContext  int     `json:"after_context"`
		Include       string  `json:"include"`
		Exclude       string  `json:"exclude"`
	}

	err := json.Unmarshal([]byte(argStr), &args)
	if err != nil {
		logger.L().Error().Err(err).Msg("解析失败")
		return nil, fmt.Errorf("解析失败: %w", err)
	}

	if args.Pattern == "" {
		return nil, fmt.Errorf("缺少必需参数: pattern")
	}

	// 设置默认值
	if args.Path == "" {
		args.Path = "."
	}
	if !args.Recursive {
		args.Recursive = true
	}
	if !args.ShowLineNum {
		args.ShowLineNum = true
	}

	// 编译正则表达式（如果需要）
	var re *regexp.Regexp
	if args.IsRegex {
		re, err = regexp.Compile(args.Pattern)
		if err != nil {
			return nil, fmt.Errorf("正则表达式编译失败: %w", err)
		}
	}

	// 收集要搜索的文件
	files, err := t.collectFiles(args.Path, args.Recursive, args.Include, args.Exclude)
	if err != nil {
		return nil, err
	}

	// 搜索每个文件
	var matches []GrepMatch
	for _, file := range files {
		fileMatches, err := t.searchFile(file, args.Pattern, re, args.ShowLineNum, args.BeforeContext, args.AfterContext)
		if err != nil {
			logger.L().Error().Err(err).Str("file", file).Msg("搜索文件失败")
			continue
		}
		matches = append(matches, fileMatches...)
	}

	return matches, nil
}

// collectFiles 收集要搜索的文件
func (t *GrepTool) collectFiles(basePath string, recursive bool, include string, exclude string) ([]string, error) {
	var files []string

	// 检查路径是文件还是目录
	info, err := os.Stat(basePath)
	if err != nil {
		return nil, fmt.Errorf("路径不存在: %w", err)
	}

	if !info.IsDir() {
		// 单个文件
		if t.matchFile(basePath, include, exclude) {
			files = append(files, basePath)
		}
		return files, nil
	}

	// 目录
	var pattern string
	if recursive {
		pattern = "**/*"
	} else {
		pattern = "*"
	}

	fs := os.DirFS(basePath)
	allFiles, err := doublestar.Glob(fs, pattern)
	if err != nil {
		return nil, fmt.Errorf("收集文件失败: %w", err)
	}

	for _, f := range allFiles {
		fullPath := filepath.Join(basePath, f)
		info, err := os.Stat(fullPath)
		if err != nil || info.IsDir() {
			continue
		}
		if t.matchFile(f, include, exclude) {
			files = append(files, fullPath)
		}
	}

	return files, nil
}

// matchFile 检查文件是否匹配包含/排除模式
func (t *GrepTool) matchFile(path string, include string, exclude string) bool {
	filename := filepath.Base(path)

	// 先检查排除
	if exclude != "" {
		matched, _ := doublestar.Match(exclude, filename)
		if matched {
			return false
		}
	}

	// 再检查包含
	if include != "" {
		matched, _ := doublestar.Match(include, filename)
		return matched
	}

	return true
}

// searchFile 搜索单个文件
func (t *GrepTool) searchFile(path string, pattern string, re *regexp.Regexp, showLineNum bool, beforeCtx, afterCtx int) ([]GrepMatch, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	var matches []GrepMatch
	for i, line := range lines {
		var matched bool
		if re != nil {
			matched = re.MatchString(line)
		} else {
			matched = contains(line, pattern)
		}

		if matched {
			match := GrepMatch{
				File:    path,
				Content: line,
			}
			if showLineNum {
				match.Line = i + 1
			}

			// 添加前导上下文
			if beforeCtx > 0 {
				start := max(0, i-beforeCtx)
				match.Before = lines[start:i]
			}

			// 添加后导上下文
			if afterCtx > 0 {
				end := min(len(lines), i+1+afterCtx)
				match.After = lines[i+1 : end]
			}

			matches = append(matches, match)
		}
	}

	return matches, nil
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (len(substr) == 0 || indexOf(s, substr) != -1)
}

func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		match := true
		for j := 0; j < len(substr); j++ {
			if s[i+j] != substr[j] {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
