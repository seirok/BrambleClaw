package hook

import (
	"context"
	"errors"
	"fmt"
	"neoclaw/internal/interfaces"
	"sync"
)

// internalHookEntry 内部 Hook 注册表的值类型
// 包含一个钩子点的所有内部 Hook 定义
type internalHookEntry struct {
	Point       string
	Definitions []HookDefinition
}

// InternalHookRegistry 内部 Hook 注册表，实现 interfaces.Registry[*internalHookEntry]
// key 为钩子点名称（如 "order.before_save"），value 为该点的 Hook 定义集合
type InternalHookRegistry struct {
	mu   sync.RWMutex
	data map[string]*internalHookEntry
}

// 编译时检查接口实现
var _ interfaces.Registry[*internalHookEntry] = (*InternalHookRegistry)(nil)

// NewInternalHookRegistry 创建新的内部 Hook 注册表
func NewInternalHookRegistry() *InternalHookRegistry {
	return &InternalHookRegistry{
		data: make(map[string]*internalHookEntry),
	}
}

// Register 注册一个内部 Hook 条目
func (r *InternalHookRegistry) Register(ctx context.Context, name string, value *internalHookEntry) error {
	if name == "" {
		return errors.New("hook point name cannot be empty")
	}
	if value == nil {
		return errors.New("hook entry cannot be nil")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.data[name]; exists {
		return fmt.Errorf("internal hook entry already exists: %s", name)
	}

	r.data[name] = value
	return nil
}

// Unregister 注销一个内部 Hook 条目
func (r *InternalHookRegistry) Unregister(ctx context.Context, name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.data[name]; !exists {
		return fmt.Errorf("internal hook entry not found: %s", name)
	}

	delete(r.data, name)
	return nil
}

// Get 获取一个内部 Hook 条目
func (r *InternalHookRegistry) Get(ctx context.Context, name string) (*internalHookEntry, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	entry, ok := r.data[name]
	if !ok {
		return nil, fmt.Errorf("internal hook entry not found: %s", name)
	}
	return entry, nil
}

// List 列出所有内部 Hook 条目
func (r *InternalHookRegistry) List(ctx context.Context) []*internalHookEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*internalHookEntry, 0, len(r.data))
	for _, entry := range r.data {
		result = append(result, entry)
	}
	return result
}

// GetOrCreate 获取或创建内部 Hook 条目（扩展方法）
func (r *InternalHookRegistry) GetOrCreate(point string) *internalHookEntry {
	r.mu.Lock()
	defer r.mu.Unlock()

	if entry, exists := r.data[point]; exists {
		return entry
	}

	entry := &internalHookEntry{
		Point:       point,
		Definitions: make([]HookDefinition, 0),
	}
	r.data[point] = entry
	return entry
}

// AddDefinition 向指定钩子点添加 Hook 定义（扩展方法）
func (r *InternalHookRegistry) AddDefinition(point string, def HookDefinition) {
	r.mu.Lock()
	defer r.mu.Unlock()

	entry, exists := r.data[point]
	if !exists {
		entry = &internalHookEntry{
			Point:       point,
			Definitions: make([]HookDefinition, 0),
		}
		r.data[point] = entry
	}

	entry.Definitions = append(entry.Definitions, def)
}

// GetDefinitions 获取指定钩子点的所有 Hook 定义（扩展方法）
func (r *InternalHookRegistry) GetDefinitions(point string) []HookDefinition {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if entry, exists := r.data[point]; exists {
		return entry.Definitions
	}
	return nil
}

// Points 列出所有已注册的钩子点名称（扩展方法）
func (r *InternalHookRegistry) Points() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	points := make([]string, 0, len(r.data))
	for point := range r.data {
		points = append(points, point)
	}
	return points
}

// --- 外部 Hook 注册表 ---

// ExternalHookRegistry 外部 Hook 注册表，实现 interfaces.Registry[*ExternalHook]
// key 为 "point:scriptPath" 组合键，确保唯一性
type ExternalHookRegistry struct {
	mu   sync.RWMutex
	data map[string]*ExternalHook
}

// 编译时检查接口实现
var _ interfaces.Registry[*ExternalHook] = (*ExternalHookRegistry)(nil)

// NewExternalHookRegistry 创建新的外部 Hook 注册表
func NewExternalHookRegistry() *ExternalHookRegistry {
	return &ExternalHookRegistry{
		data: make(map[string]*ExternalHook),
	}
}

// externalRegistryKey 生成注册表 key
func externalRegistryKey(point, scriptPath string) string {
	return point + ":" + scriptPath
}

// Register 注册一个外部 Hook
func (r *ExternalHookRegistry) Register(ctx context.Context, name string, value *ExternalHook) error {
	if name == "" {
		return errors.New("external hook key cannot be empty")
	}
	if value == nil {
		return errors.New("external hook cannot be nil")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.data[name]; exists {
		return fmt.Errorf("external hook already exists: %s", name)
	}

	r.data[name] = value
	return nil
}

// Unregister 注销一个外部 Hook
func (r *ExternalHookRegistry) Unregister(ctx context.Context, name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.data[name]; !exists {
		return fmt.Errorf("external hook not found: %s", name)
	}

	delete(r.data, name)
	return nil
}

// Get 获取一个外部 Hook
func (r *ExternalHookRegistry) Get(ctx context.Context, name string) (*ExternalHook, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	hook, ok := r.data[name]
	if !ok {
		return nil, fmt.Errorf("external hook not found: %s", name)
	}
	return hook, nil
}

// List 列出所有外部 Hook
func (r *ExternalHookRegistry) List(ctx context.Context) []*ExternalHook {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*ExternalHook, 0, len(r.data))
	for _, hook := range r.data {
		result = append(result, hook)
	}
	return result
}

// AddHook 添加外部 Hook（扩展方法，自动生成 key）
func (r *ExternalHookRegistry) AddHook(hook *ExternalHook) {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := externalRegistryKey(hook.Point(), hook.config.ScriptPath)
	r.data[key] = hook
}

// RemoveHook 移除外部 Hook（扩展方法）
func (r *ExternalHookRegistry) RemoveHook(point, scriptPath string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := externalRegistryKey(point, scriptPath)
	if _, exists := r.data[key]; !exists {
		return fmt.Errorf("external hook not found: %s", key)
	}

	delete(r.data, key)
	return nil
}

// GetByPoint 获取指定钩子点的所有外部 Hook（扩展方法）
func (r *ExternalHookRegistry) GetByPoint(point string) []*ExternalHook {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*ExternalHook
	for _, hook := range r.data {
		if hook.Point() == point {
			result = append(result, hook)
		}
	}
	return result
}

// Points 列出所有有外部 Hook 的钩子点名称（扩展方法）
func (r *ExternalHookRegistry) Points() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	pointSet := make(map[string]bool)
	for _, hook := range r.data {
		pointSet[hook.Point()] = true
	}

	points := make([]string, 0, len(pointSet))
	for point := range pointSet {
		points = append(points, point)
	}
	return points
}
