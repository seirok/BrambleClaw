package hook

import (
	"context"
	"fmt"
	"reflect"
	"sync"
	"time"
)

// HookFunc 定义钩子函数签名
// ctx: 钩子执行上下文
// input: 传入的数据
// output: 处理后返回的数据（用于流水线修改）
type HookFunc func(ctx context.Context, input any) (any, error)

// Priority 定义钩子优先级，值越小优先级越高
type Priority int

const (
	PriorityHigh   Priority = 10
	PriorityNormal Priority = 50
	PriorityLow    Priority = 100
)

// ErrorStrategy 定义钩子错误处理策略
type ErrorStrategy int

const (
	ErrorStrategyStop     ErrorStrategy = iota // 停止策略：遇到错误立即停止（默认行为）
	ErrorStrategyContinue                      // 继续策略：跳过失败的钩子，继续执行后续钩子
	ErrorStrategyIgnore                        // 忽略策略：完全忽略错误，继续执行
)

// HookDefinition 包含钩子的完整信息
type HookDefinition struct {
	Func     HookFunc
	Priority Priority
	Index    int
}

// HookMetrics 跟踪钩子执行指标
type HookMetrics struct {
	Point          string
	ExecutionCount int64
	TotalDuration  time.Duration
	ErrorCount     int64
	LastExecution  time.Time
}

// HookError 包含钩子错误信息
type HookError struct {
	Point    string
	Index    int
	Priority Priority
	Input    any
	Err      error
}

func (e *HookError) Error() string {
	return fmt.Sprintf("hook [%s] at index %d (priority %d) failed: %v", e.Point, e.Index, e.Priority, e.Err)
}

// --- 旧的全局 engine（仅用于内部 Hook 的包级快捷函数） ---

type legacyEngine struct {
	mu      sync.RWMutex
	hooks   map[string][]HookDefinition
	metrics map[string]*HookMetrics
	debug   bool
}

var globalEngine = &legacyEngine{
	hooks:   make(map[string][]HookDefinition),
	metrics: make(map[string]*HookMetrics),
	debug:   true,
}

// Register 注册钩子（默认优先级）
// 委托到全局 HookEngine
func Register(point string, fn HookFunc) {
	GetEngine().Register(point, fn)
}

// RegisterWithPriority 注册带优先级的钩子
// 委托到全局 HookEngine
func RegisterWithPriority(point string, priority Priority, fn HookFunc) {
	GetEngine().RegisterWithPriority(point, priority, fn)
}

// Unregister 注销特定钩子
// 委托到全局 HookEngine
func Unregister(point string, fn HookFunc) error {
	return GetEngine().Unregister(point, fn)
}

// Clear 清除特定钩子点的所有钩子
func Clear(point string) {
	GetEngine().UnregisterExternal(point, "")
}

// List 列出所有已注册的钩子点
// 委托到全局 HookEngine
func List() []string {
	return GetEngine().List()
}

// Emit 触发钩子（流水线模式）
// 委托到全局 HookEngine
func Emit(ctx context.Context, point string, data any) (any, error) {
	return GetEngine().Emit(ctx, point, data)
}

// EmitWithStrategy 触发钩子并指定错误策略
// 委托到全局 HookEngine
func EmitWithStrategy(ctx context.Context, point string, data any, strategy ErrorStrategy) (any, []error) {
	return GetEngine().EmitWithStrategy(ctx, point, data, strategy)
}

// MustEmit 触发钩子，如果出错则记录错误并返回 nil
func MustEmit(ctx context.Context, point string, data any) any {
	res, err := Emit(ctx, point, data)
	if err != nil {
		return nil
	}
	return res
}

// Metrics 获取钩子执行指标
func Metrics(point string) *HookMetrics {
	globalEngine.mu.RLock()
	defer globalEngine.mu.RUnlock()

	if metrics, exists := globalEngine.metrics[point]; exists {
		return &HookMetrics{
			Point:          metrics.Point,
			ExecutionCount: metrics.ExecutionCount,
			TotalDuration:  metrics.TotalDuration,
			ErrorCount:     metrics.ErrorCount,
			LastExecution:  metrics.LastExecution,
		}
	}

	return nil
}

// Count 返回钩子点的钩子数量
// 委托到全局 HookEngine
func Count(point string) int {
	return GetEngine().Count(point)
}

// SetDebugEnabled 设置是否启用调试日志
func SetDebugEnabled(enabled bool) {
	GetEngine().SetDebugEnabled(enabled)
}

// ResetMetrics 重置所有钩子点的指标
func ResetMetrics() {
	globalEngine.mu.Lock()
	defer globalEngine.mu.Unlock()

	for point := range globalEngine.metrics {
		globalEngine.metrics[point] = &HookMetrics{
			Point: point,
		}
	}
}

// --- legacyEngine 内部方法（仅 Metrics 和 ResetMetrics 使用） ---

// registerLegacy 内部注册方法，同步到 legacyEngine 的 metrics
func registerLegacy(point string, fn HookFunc, priority Priority) {
	globalEngine.mu.Lock()
	defer globalEngine.mu.Unlock()

	if _, exists := globalEngine.hooks[point]; !exists {
		globalEngine.hooks[point] = make([]HookDefinition, 0)
		globalEngine.metrics[point] = &HookMetrics{Point: point}
	}

	_ = reflect.ValueOf(fn).Pointer() // 用于兼容旧代码
}
