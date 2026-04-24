package tools

import (
	"brambleclaw/internal/logger"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// FileSystemTool 文件系统工具
type FileSystemTool struct {
	name        string
	description string
}

// NewFileSystemTool 创建文件系统工具
func NewFileSystemTool() *FileSystemTool {
	return &FileSystemTool{
		name:        "FileSystem",
		description: "文件系统操作工具，用于读写文件",
	}
}

// Name 返回工具名称
func (t *FileSystemTool) Name() string {
	return t.name
}

// Description 返回工具描述
func (t *FileSystemTool) Description() string {
	return t.description
}

func (t *FileSystemTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"command": map[string]interface{}{
				"type":        "string",
				"description": "操作命令: read, write, list",
			},
			"path": map[string]interface{}{
				"type":        "string",
				"description": "文件或目录路径",
			},
			"content": map[string]interface{}{
				"type":        "string",
				"description": "文件内容（仅write命令需要）",
			},
		},
		"required": []string{"command"},
	}
}

// Execute 执行工具
func (t *FileSystemTool) Execute(ctx context.Context, argStr string) (interface{}, error) {
	var args map[string]interface{}
	err := json.Unmarshal([]byte(argStr), &args)
	if err != nil {
		logger.L().Fatal().Err(err).Msg("解析失败")
		return nil, fmt.Errorf("解析失败: %w", err)
	}
	cmd, ok := args["command"].(string)
	if !ok {
		logger.L().Fatal().Msg("参数解析有误")
		return nil, fmt.Errorf("参数解析有误")
	}

	switch cmd {
	case "read":
		return t.readFile(args)
	case "write":
		return t.writeFile(args)
	case "list":
		return t.listFiles(args)
	default:
		return nil, nil
	}
}

// readFile 读取文件
func (t *FileSystemTool) readFile(args map[string]interface{}) (interface{}, error) {
	path, ok := args["path"].(string)
	if !ok {
		return nil, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取文件失败(%s): %w", path, err)
	}

	return string(data), nil
}

// writeFile 写入文件
func (t *FileSystemTool) writeFile(args map[string]interface{}) (interface{}, error) {
	path, ok := args["path"].(string)
	if !ok {
		return nil, nil
	}

	content, ok := args["content"].(string)
	if !ok {
		return nil, nil
	}

	// 确保目录存在
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("创建目录失败(%s): %w", dir, err)
	}

	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return nil, fmt.Errorf("写入文件失败(%s): %w", path, err)
	}

	return "File written successfully", nil
}

// listFiles 列出文件
func (t *FileSystemTool) listFiles(args map[string]interface{}) (interface{}, error) {
	path, ok := args["path"].(string)
	if !ok {
		path = "."
	}

	files, err := os.ReadDir(path)
	if err != nil {
		return nil, fmt.Errorf("读取目录失败(%s): %w", path, err)
	}

	fileList := make([]map[string]interface{}, 0, len(files))
	for _, file := range files {
		info, err := file.Info()
		if err != nil {
			logger.L().Error().Err(err).Str("File", file.Name()).Msg("获取文件信息失败")
			continue
		}
		fileList = append(fileList, map[string]interface{}{
			"name":     info.Name(),
			"size":     info.Size(),
			"is_dir":   info.IsDir(),
			"mod_time": info.ModTime(),
		})
	}

	return fileList, nil
}
