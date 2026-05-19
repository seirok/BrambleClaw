package validation

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/google/jsonschema-go/jsonschema"
	"neoclaw/internal/logger"
)

// ValidationError 表示参数校验失败错误
type ValidationError struct {
	ToolName    string // 工具名称
	Args        string // 原始参数 JSON
	SchemaError string // 具体的校验错误信息
}

// Error 实现 error 接口
func (e *ValidationError) Error() string {
	return fmt.Sprintf("参数校验失败: %s", e.SchemaError)
}

// ToLLMMessage 生成适合 LLM 读取的错误消息
func (e *ValidationError) ToLLMMessage() string {
	return fmt.Sprintf(
		"Parameter validation failed for tool %q: %s. The arguments you provided were: %s. Please correct the parameters according to the tool's schema and try again.",
		e.ToolName,
		e.SchemaError,
		e.Args,
	)
}

// SchemaCache 线程安全的已解析 schema 缓存
type SchemaCache struct {
	mu       sync.RWMutex
	resolved map[string]*jsonschema.Resolved
}

// NewSchemaCache 创建一个新的 schema 缓存
func NewSchemaCache() *SchemaCache {
	return &SchemaCache{
		resolved: make(map[string]*jsonschema.Resolved),
	}
}

// GetOrResolve 获取或解析并缓存工具的 schema
func (c *SchemaCache) GetOrResolve(toolName string, params map[string]interface{}) (*jsonschema.Resolved, error) {
	c.mu.RLock()
	if r, ok := c.resolved[toolName]; ok {
		c.mu.RUnlock()
		return r, nil
	}
	c.mu.RUnlock()

	// 将 map 序列化为 JSON
	schemaBytes, err := json.Marshal(params)
	if err != nil {
		return nil, fmt.Errorf("序列化 schema 失败: %w", err)
	}

	// 反序列化为 jsonschema.Schema
	var schema jsonschema.Schema
	if err := json.Unmarshal(schemaBytes, &schema); err != nil {
		return nil, fmt.Errorf("解析 schema 失败: %w", err)
	}

	// 解析 schema
	resolved, err := schema.Resolve(nil)
	if err != nil {
		return nil, fmt.Errorf("解析 schema 失败: %w", err)
	}

	// 缓存结果
	c.mu.Lock()
	c.resolved[toolName] = resolved
	c.mu.Unlock()

	return resolved, nil
}

// Invalidate 移除某个工具的 schema 缓存
func (c *SchemaCache) Invalidate(toolName string) {
	c.mu.Lock()
	delete(c.resolved, toolName)
	c.mu.Unlock()
}

// Validate 校验 LLM 生成的参数是否符合工具的 schema
func Validate(toolName string, params map[string]interface{}, argsJSON string, cache *SchemaCache) *ValidationError {
	// 如果无 schema，跳过校验
	if params == nil {
		return nil
	}

	// 获取或解析 schema
	if cache == nil {
		cache = NewSchemaCache()
	}
	resolved, err := cache.GetOrResolve(toolName, params)
	if err != nil {
		// schema 解析失败，跳过校验，记录警告
		logger.L().Warn().Err(err).Str("tool", toolName).Msg("无法解析工具 schema，跳过参数校验")
		return nil
	}

	// 解析参数 JSON，空字符串视为 {}
	var args map[string]interface{}
	if argsJSON == "" {
		args = make(map[string]interface{})
	} else {
		if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
			return &ValidationError{
				ToolName:    toolName,
				Args:        argsJSON,
				SchemaError: fmt.Sprintf("invalid JSON: %s", err.Error()),
			}
		}
	}

	// 校验参数
	if err := resolved.Validate(args); err != nil {
		return &ValidationError{
			ToolName:    toolName,
			Args:        argsJSON,
			SchemaError: err.Error(),
		}
	}

	return nil
}
