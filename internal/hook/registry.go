package hook

import (
	"context"
	"errors"
	"fmt"
	"neoclaw/internal/interfaces"
	"neoclaw/internal/registry"
)

// internalHookEntry 内部 Hook 注册表的值类型
// 包含一个钩子点的所有 Hook 定义
type internalHookEntry struct {
	Point       string
	Definitions []HookDefinition
}

// InternalHookRegistry 内部 Hook 注册表，实现 interfaces.Registry[*internalHookEntry]
// key 为钩子点名称（如 "order.before_save"），value 为该点的 Hook 定义集合
type InternalHookRegistry struct {
	*registry.GenericRegistry[*internalHookEntry]
}

// 编译时检查接口实现
var _ interfaces.Registry[*internalHookEntry] = (*InternalHookRegistry)(nil)

// NewInternalHookRegistry 创建新的内部 Hook 注册表
func NewInternalHookRegistry() *InternalHookRegistry {
	return &InternalHookRegistry{
		GenericRegistry: registry.NewGenericRegistry[*internalHookEntry](
			func(name string) error { return fmt.Errorf("internal hook entry already exists: %s", name) },
			func(name string) error { return fmt.Errorf("internal hook entry not found: %s", name) },
			func(name string, value *internalHookEntry) error {
				if name == "" {
					return errors.New("hook point name cannot be empty")
				}
				if value == nil {
					return errors.New("hook entry cannot be nil")
				}
				return nil
			},
		),
	}
}

// GetOrCreate 获取或创建内部 Hook 条目（扩展方法）
func (r *InternalHookRegistry) GetOrCreate(point string) *internalHookEntry {
	var entry *internalHookEntry
	r.Read(func(items map[string]*internalHookEntry) {
		entry = items[point]
	})
	if entry != nil {
		return entry
	}

	entry = &internalHookEntry{
		Point:       point,
		Definitions: make([]HookDefinition, 0),
	}
	r.Set(point, entry)
	return entry
}

// AddDefinition 向指定钩子点添加 Hook 定义（扩展方法）
func (r *InternalHookRegistry) AddDefinition(point string, def HookDefinition) {
	r.Mutate(func(items map[string]*internalHookEntry) {
		entry, exists := items[point]
		if !exists {
			entry = &internalHookEntry{
				Point:       point,
				Definitions: make([]HookDefinition, 0),
			}
			items[point] = entry
		}
		entry.Definitions = append(entry.Definitions, def)
	})
}

// GetDefinitions 获取指定钩子点的所有 Hook 定义（扩展方法）
func (r *InternalHookRegistry) GetDefinitions(point string) []HookDefinition {
	var result []HookDefinition
	r.Read(func(items map[string]*internalHookEntry) {
		if entry, exists := items[point]; exists {
			result = entry.Definitions
		}
	})
	return result
}

// Points 列出所有已注册的钩子点名称（扩展方法）
func (r *InternalHookRegistry) Points() []string {
	var result []string
	r.Read(func(items map[string]*internalHookEntry) {
		result = make([]string, 0, len(items))
		for point := range items {
			result = append(result, point)
		}
	})
	return result
}

// --- 外部 Hook 注册表 ---

// ExternalHookRegistry 外部 Hook 注册表，实现 interfaces.Registry[*ExternalHook]
// key 为 "point:scriptPath" 组合键，确保唯一性
type ExternalHookRegistry struct {
	*registry.GenericRegistry[*ExternalHook]
}

// 编译时检查接口实现
var _ interfaces.Registry[*ExternalHook] = (*ExternalHookRegistry)(nil)

// NewExternalHookRegistry 创建新的外部 Hook 注册表
func NewExternalHookRegistry() *ExternalHookRegistry {
	return &ExternalHookRegistry{
		GenericRegistry: registry.NewGenericRegistry[*ExternalHook](
			func(name string) error { return fmt.Errorf("external hook already exists: %s", name) },
			func(name string) error { return fmt.Errorf("external hook not found: %s", name) },
			func(name string, value *ExternalHook) error {
				if name == "" {
					return errors.New("external hook key cannot be empty")
				}
				if value == nil {
					return errors.New("external hook cannot be nil")
				}
				return nil
			},
		),
	}
}

// externalRegistryKey 生成注册表 key
func externalRegistryKey(point, scriptPath string) string {
	return point + ":" + scriptPath
}

// AddHook 添加外部 Hook（扩展方法，自动生成 key）
func (r *ExternalHookRegistry) AddHook(hook *ExternalHook) {
	key := externalRegistryKey(hook.Point(), hook.config.ScriptPath)
	r.Set(key, hook)
}

// RemoveHook 移除外部 Hook（扩展方法）
func (r *ExternalHookRegistry) RemoveHook(point, scriptPath string) error {
	key := externalRegistryKey(point, scriptPath)
	ctx := context.Background()
	return r.Unregister(ctx, key)
}

// GetByPoint 获取指定钩子点的所有外部 Hook（扩展方法）
func (r *ExternalHookRegistry) GetByPoint(point string) []*ExternalHook {
	var result []*ExternalHook
	r.Read(func(items map[string]*ExternalHook) {
		for _, hook := range items {
			if hook.Point() == point {
				result = append(result, hook)
			}
		}
	})
	return result
}

// Points 列出所有有外部 Hook 的钩子点名称（扩展方法）
func (r *ExternalHookRegistry) Points() []string {
	var result []string
	r.Read(func(items map[string]*ExternalHook) {
		pointSet := make(map[string]bool)
		for _, hook := range items {
			pointSet[hook.Point()] = true
		}
		result = make([]string, 0, len(pointSet))
		for point := range pointSet {
			result = append(result, point)
		}
	})
	return result
}
