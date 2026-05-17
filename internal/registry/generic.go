package registry

import (
	"context"
	"fmt"
	"sync"
)

// GenericRegistry 是一个通用的 Registry 实现，可被其他特定 Registry 嵌入使用
type GenericRegistry[V any] struct {
	mu        sync.RWMutex
	items     map[string]V
	errExists func(name string) error
	errNotFound func(name string) error
	validate  func(name string, value V) error
}

// NewGenericRegistry 创建一个通用 Registry
//
// errExists: 当条目已存在时返回的错误构造函数，为 nil 则使用默认错误
// errNotFound: 当条目不存在时返回的错误构造函数，为 nil 则使用默认错误
// validate: 注册前的自定义验证函数，为 nil 则跳过
func NewGenericRegistry[V any](
	errExists func(name string) error,
	errNotFound func(name string) error,
	validate func(name string, value V) error,
) *GenericRegistry[V] {
	return &GenericRegistry[V]{
		items:     make(map[string]V),
		errExists: errExists,
		errNotFound: errNotFound,
		validate:  validate,
	}
}

// Register 实现 interfaces.Registry.Register
func (r *GenericRegistry[V]) Register(ctx context.Context, name string, value V) error {
	if r.validate != nil {
		if err := r.validate(name, value); err != nil {
			return err
		}
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.items[name]; exists {
		if r.errExists != nil {
			return r.errExists(name)
		}
		return fmt.Errorf("item already exists: %s", name)
	}

	r.items[name] = value
	return nil
}

// Unregister 实现 interfaces.Registry.Unregister
func (r *GenericRegistry[V]) Unregister(ctx context.Context, name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.items[name]; !exists {
		if r.errNotFound != nil {
			return r.errNotFound(name)
		}
		return fmt.Errorf("item not found: %s", name)
	}

	delete(r.items, name)
	return nil
}

// Get 实现 interfaces.Registry.Get
func (r *GenericRegistry[V]) Get(ctx context.Context, name string) (V, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	item, ok := r.items[name]
	if !ok {
		var zero V
		if r.errNotFound != nil {
			return zero, r.errNotFound(name)
		}
		return zero, fmt.Errorf("item not found: %s", name)
	}
	return item, nil
}

// List 实现 interfaces.Registry.List
func (r *GenericRegistry[V]) List(ctx context.Context) []V {
	r.mu.RLock()
	defer r.mu.RUnlock()

	list := make([]V, 0, len(r.items))
	for _, item := range r.items {
		list = append(list, item)
	}
	return list
}

// --- 以下是扩展方法（非接口），方便嵌入使用 ---

// Has 检查条目是否存在
func (r *GenericRegistry[V]) Has(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, exists := r.items[name]
	return exists
}

// Set 设置或覆盖条目（不检查是否已存在）
func (r *GenericRegistry[V]) Set(name string, value V) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.items[name] = value
}

// GetInternalMap 获取内部 map 引用（仅用于序列化/反序列化场景，谨慎使用）
func (r *GenericRegistry[V]) GetInternalMap() map[string]V {
	return r.items
}

// Update 更新条目，不存在则返回错误
func (r *GenericRegistry[V]) Update(ctx context.Context, name string, value V) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.items[name]; !exists {
		if r.errNotFound != nil {
			return r.errNotFound(name)
		}
		return fmt.Errorf("item not found: %s", name)
	}

	r.items[name] = value
	return nil
}

// Lock 加写锁（供扩展方法在需要时使用，必须小心配对 Unlock）
func (r *GenericRegistry[V]) Lock() {
	r.mu.Lock()
}

// Unlock 解写锁
func (r *GenericRegistry[V]) Unlock() {
	r.mu.Unlock()
}

// RLock 加读锁（供扩展方法在需要时使用，必须小心配对 RUnlock）
func (r *GenericRegistry[V]) RLock() {
	r.mu.RLock()
}

// RUnlock 解读锁
func (r *GenericRegistry[V]) RUnlock() {
	r.mu.RUnlock()
}

// Mutate 在持有写锁的情况下执行自定义操作
func (r *GenericRegistry[V]) Mutate(fn func(items map[string]V)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	fn(r.items)
}

// Read 在持有读锁的情况下执行自定义操作
func (r *GenericRegistry[V]) Read(fn func(items map[string]V)) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	fn(r.items)
}
