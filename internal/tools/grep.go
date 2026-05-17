package tools

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/bmatcuk/doublestar/v4"
)

// GrepMatch 单个匹配结果
type GrepMatch struct {
	File    string   `json:"file"`
	Line    int      `json:"line,omitempty"`
	Content string   `json:"content"`
	Before  []string `json:"before,omitempty"`
	After   []string `json:"after,omitempty"`
}

// GrepTool 文件内容搜索工具
type GrepTool struct {
	*BaseTool
}

// NewGrepTool 创建 GrepTool
func NewGrepTool() *GrepTool {
	return &GrepTool{
		BaseTool: NewBaseTool(
			"grep",
			"文件内容搜索工具，支持字符串匹配和正则表达式",
			nil,
			map[string]any{
				"type": "object",
				"properties": map[string]any{
					"pattern": map[string]any{
						"type":        "string",
						"description": "搜索模式（字符串或正则表达式）",
					},
					"path": map[string]any{
						"type":        "string",
						"description": "搜索路径（可选，默认当前目录）",
					},
					"is_regex": map[string]any{
						"type":        "boolean",
						"description": "是否使用正则表达式（可选，默认 false）",
					},
					"recursive": map[string]any{
						"type":        "boolean",
						"description": "是否递归搜索子目录（可选，默认 true）",
					},
					"show_line_num": map[string]any{
						"type":        "boolean",
						"description": "是否显示行号（可选，默认 true）",
					},
					"before_context": map[string]any{
						"type":        "number",
						"description": "匹配前显示的行数（可选，默认 0）",
					},
					"after_context": map[string]any{
						"type":        "number",
						"description": "匹配后显示的行数（可选，默认 0）",
					},
					"include": map[string]any{
						"type":        "string",
						"description": "包含的文件模式（可选）",
					},
					"exclude": map[string]any{
						"type":        "string",
						"description": "排除的文件模式（可选）",
					},
				},
				"required": []string{"pattern"},
			},
		),
	}
}

// Execute 执行工具
func (t *GrepTool) Execute(ctx context.Context, argStr string) (any, error) {
	t.LogStart()
	var args struct {
		Pattern       string `json:"pattern"`
		Path          string `json:"path"`
		IsRegex       bool   `json:"is_regex"`
		Recursive     bool   `json:"recursive"`
		ShowLineNum   bool   `json:"show_line_num"`
		BeforeContext int    `json:"before_context"`
		AfterContext  int    `json:"after_context"`
		Include       string `json:"include"`
		Exclude       string `json:"exclude"`
	}

	err := json.Unmarshal([]byte(argStr), &args)
	if err != nil {
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
			continue
		}
		matches = append(matches, fileMatches...)
	}

	return matches, nil
}

func (t *GrepTool) collectFiles(basePath string, recursive bool, include string, exclude string) ([]string, error) {
	var files []string
	info, err := os.Stat(basePath)
	if err != nil {
		return nil, fmt.Errorf("路径不存在: %w", err)
	}

	if !info.IsDir() {
		if t.matchFile(basePath, include, exclude) {
			files = append(files, basePath)
		}
		return files, nil
	}

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

func (t *GrepTool) matchFile(path string, include string, exclude string) bool {
	filename := filepath.Base(path)
	if exclude != "" {
		matched, _ := doublestar.Match(exclude, filename)
		if matched {
			return false
		}
	}

	if include != "" {
		matched, _ := doublestar.Match(include, filename)
		return matched
	}

	return true
}

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

			if beforeCtx > 0 {
				start := max(0, i-beforeCtx)
				match.Before = lines[start:i]
			}
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
