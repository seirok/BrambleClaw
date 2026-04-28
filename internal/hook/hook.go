package hook

import (
	"brambleclaw/internal/logger"
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
type HookFunc func(ctx context.Context, input interface{}) (interface{}, error)

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

// engine 内部钩子引擎实现
type engine struct {
	mu      sync.RWMutex
	hooks   map[string][]HookDefinition
	metrics map[string]*HookMetrics
	debug   bool
}

// HookMetrics 跟踪钩子执行指标
type HookMetrics struct {
	Point          string
	ExecutionCount int64
	TotalDuration  time.Duration
	ErrorCount     int64
	LastExecution  time.Time
}

// 全局单例，方便随处调用
var globalEngine = &engine{
	hooks:   make(map[string][]HookDefinition),
	metrics: make(map[string]*HookMetrics),
	debug:   true,
}

// Register 注册钩子（默认优先级）
// point: 钩子点名称，建议使用 "module.action" 格式
func Register(point string, fn HookFunc) {
	RegisterWithPriority(point, PriorityNormal, fn)
}

// RegisterWithPriority 注册带优先级的钩子
func RegisterWithPriority(point string, priority Priority, fn HookFunc) {
	globalEngine.mu.Lock()
	defer globalEngine.mu.Unlock()

	if _, exists := globalEngine.hooks[point]; !exists {
		globalEngine.hooks[point] = make([]HookDefinition, 0)
		globalEngine.metrics[point] = &HookMetrics{
			Point: point,
		}
	}

	// 创建钩子定义
	hookDef := HookDefinition{
		Func:     fn,
		Priority: priority,
		Index:    len(globalEngine.hooks[point]),
	}

	// 按优先级插入到正确位置（保持有序）
	insertIndex := 0
	for i, existing := range globalEngine.hooks[point] {
		if priority < existing.Priority {
			insertIndex = i
			break
		}
		insertIndex = i + 1
	}

	// 插入到切片中
	globalEngine.hooks[point] = append(
		globalEngine.hooks[point][:insertIndex],
		append([]HookDefinition{hookDef}, globalEngine.hooks[point][insertIndex:]...)...,
	)

	// 更新索引
	for i := insertIndex + 1; i < len(globalEngine.hooks[point]); i++ {
		globalEngine.hooks[point][i].Index = i
	}

	if globalEngine.debug {
		logger.L().Debug().
			Str("hook_point", point).
			Int("priority", int(priority)).
			Str("func_type", reflect.TypeOf(fn).Name()).
			Msg("Hook registered successfully")
	}
}

// Unregister 注销特定钩子
func Unregister(point string, fn HookFunc) error {
	globalEngine.mu.Lock()
	defer globalEngine.mu.Unlock()

	if hooks, exists := globalEngine.hooks[point]; exists {
		for i, hookDef := range hooks {
			// 通过反射比较函数
			if reflect.ValueOf(hookDef.Func).Pointer() == reflect.ValueOf(fn).Pointer() {
				// 移除钩子
				globalEngine.hooks[point] = append(hooks[:i], hooks[i+1:]...)
				// 更新索引
				for j := i; j < len(globalEngine.hooks[point]); j++ {
					globalEngine.hooks[point][j].Index = j
				}

				if globalEngine.debug {
					logger.L().Debug().
						Str("hook_point", point).
						Str("func_type", reflect.TypeOf(fn).Name()).
						Msg("Hook unregistered successfully")
				}

				return nil
			}
		}
	}

	return fmt.Errorf("hook not found for point %s", point)
}

// Clear 清除特定钩子点的所有钩子
func Clear(point string) {
	globalEngine.mu.Lock()
	defer globalEngine.mu.Unlock()

	delete(globalEngine.hooks, point)
	delete(globalEngine.metrics, point)

	if globalEngine.debug {
		logger.L().Debug().
			Str("hook_point", point).
			Msg("All hooks cleared for point")
	}
}

// List 列出所有已注册的钩子点
func List() []string {
	globalEngine.mu.RLock()
	defer globalEngine.mu.RUnlock()

	points := make([]string, 0, len(globalEngine.hooks))
	for point := range globalEngine.hooks {
		points = append(points, point)
	}

	return points
}

// Emit 触发钩子（流水线模式）
// 数据会依次经过所有注册的 Hook，每个 Hook 都可以修改数据并传给下一个
func Emit(ctx context.Context, point string, data interface{}) (interface{}, error) {
	result, errors := EmitWithStrategy(ctx, point, data, ErrorStrategyStop)
	if len(errors) > 0 {
		return nil, errors[0]
	}
	return result, nil
}

// EmitWithStrategy 触发钩子并指定错误策略
func EmitWithStrategy(ctx context.Context, point string, data interface{}, strategy ErrorStrategy) (interface{}, []error) {
	globalEngine.mu.RLock()
	hooks, ok := globalEngine.hooks[point]
	globalEngine.mu.RUnlock()

	if !ok || len(hooks) == 0 {
		if globalEngine.debug {
			logger.L().Debug().
				Str("hook_point", point).
				Msg("No hooks registered for point")
		}
		return data, nil
	}

	var errors []error
	currentData := data

	for _, hookDef := range hooks {
		startTime := time.Now()

		if globalEngine.debug {
			logger.L().Debug().
				Str("hook_point", point).
				Int("hook_index", hookDef.Index).
				Int("priority", int(hookDef.Priority)).
				Str("input_type", reflect.TypeOf(currentData).Name()).
				Msg("Executing hook")
		}

		// 执行钩子
		result, err := hookDef.Func(ctx, currentData)

		// 更新指标
		duration := time.Since(startTime)
		globalEngine.mu.Lock()
		metrics := globalEngine.metrics[point]
		metrics.ExecutionCount++
		metrics.TotalDuration += duration
		metrics.LastExecution = startTime
		if err != nil {
			metrics.ErrorCount++
		}
		globalEngine.mu.Unlock()

		if err != nil {
			hookError := &HookError{
				Point:    point,
				Index:    hookDef.Index,
				Priority: hookDef.Priority,
				Input:    currentData,
				Err:      err,
			}

			if globalEngine.debug {
				logger.L().Error().
					Str("hook_point", point).
					Int("hook_index", hookDef.Index).
					Int("priority", int(hookDef.Priority)).
					Err(err).
					Msg("Hook execution failed")
			}

			switch strategy {
			case ErrorStrategyStop:
				return nil, append(errors, hookError)
			case ErrorStrategyContinue:
				errors = append(errors, hookError)
				continue // 继续执行下一个钩子
			case ErrorStrategyIgnore:
				continue // 完全忽略错误
			}
		}

		currentData = result
	}

	if len(errors) > 0 {
		return currentData, errors
	}

	return currentData, nil
}

// MustEmit 触发钩子，如果出错则 panic（适用于必须成功的初始化逻辑）
func MustEmit(ctx context.Context, point string, data interface{}) interface{} {
	res, err := Emit(ctx, point, data)
	if err != nil {
		panic(err)
	}
	return res
}

// Metrics 获取钩子执行指标
func Metrics(point string) *HookMetrics {
	globalEngine.mu.RLock()
	defer globalEngine.mu.RUnlock()

	if metrics, exists := globalEngine.metrics[point]; exists {
		// 返回副本以避免并发修改
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

// SetDebugEnabled 设置是否启用调试日志
func SetDebugEnabled(enabled bool) {
	globalEngine.debug = enabled
}

// HookError 包含钩子错误信息
type HookError struct {
	Point    string
	Index    int
	Priority Priority
	Input    interface{}
	Err      error
}

func (e *HookError) Error() string {
	return fmt.Sprintf("hook [%s] at index %d (priority %d) failed: %v", e.Point, e.Index, e.Priority, e.Err)
}

// Count 返回钩子点的钩子数量
func Count(point string) int {
	globalEngine.mu.RLock()
	defer globalEngine.mu.RUnlock()

	if hooks, exists := globalEngine.hooks[point]; exists {
		return len(hooks)
	}
	return 0
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

	logger.L().Debug().Msg("All hook metrics reset")
}
