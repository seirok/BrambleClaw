package tools

import (
	"context"
	"encoding/json"
	"log"
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
		log.Fatal("解析失败:", err)
	}
	cmd, ok := args["command"].(string)
	if !ok {
		log.Fatal("参数解析有误")
		return nil, nil
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
		return nil, err
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
		return nil, err
	}

	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return nil, err
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
		return nil, err
	}

	fileList := make([]map[string]interface{}, 0, len(files))
	for _, file := range files {
		info, err := file.Info()
		if err != nil {
			log.Println("Error:", err)
			continue
		}
		fileList = append(fileList, map[string]interface{}{
			"name":     info.Name(),
			"size":     info.Name(),
			"is_dir":   info.IsDir(),
			"mod_time": info.ModTime(),
		})
	}

	return fileList, nil
}
