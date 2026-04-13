package tools

import (
	"context"
	"log"
)

// Tool 工具接口
type Tool interface {
	// Name 返回工具名称
	Name() string

	// Description 返回工具描述
	Description() string

	// Execute 执行工具
	Execute(ctx context.Context, args string) (interface{}, error)

	Parameters() map[string]interface{}
}

// ToolRegistry 工具注册中心
type ToolRegistry struct {
	tools map[string]Tool
}

// NewToolRegistry 创建工具注册中心
func NewToolRegistry() *ToolRegistry {
	return &ToolRegistry{
		tools: make(map[string]Tool),
	}
}

// Register 注册工具
func (r *ToolRegistry) Register(tool Tool) {
	r.tools[tool.Name()] = tool
}

// Get 获取工具
func (r *ToolRegistry) Get(name string) (Tool, bool) {
	tool, ok := r.tools[name]
	if !ok {
		log.Printf("tool %s not found", name)
		return nil, false
	}
	return tool, true
}

// List 列出所有工具
func (r *ToolRegistry) List() []Tool {
	// 创建一个初始为空，但已预留好足够空间的 Tool 类型切片
	list := make([]Tool, 0, len(r.tools))
	for _, tool := range r.tools {
		list = append(list, tool)
	}
	return list
}
