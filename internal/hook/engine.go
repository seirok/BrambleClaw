package hook

import (
	"brambleclaw/internal/config/structs"
	"brambleclaw/internal/logger"
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"sync"
	"time"
)

// HookEngine 统一的 Hook 引擎接口
type HookEngine interface {
	// Register 注册内部 Go 函数 Hook
	Register(point string, fn HookFunc)

	// RegisterWithPriority 注册带优先级的内部 Hook
	RegisterWithPriority(point string, priority Priority, fn HookFunc)

	// RegisterExternal 注册外部脚本 Hook
	RegisterExternal(point string, config structs.ExternalConfig) error

	// Unregister 注销内部 Hook
	Unregister(point string, fn HookFunc) error

	// UnregisterExternal 注销外部 Hook
	UnregisterExternal(point string, scriptPath string) error

	// Emit 触发钩子（流水线模式）
	Emit(ctx context.Context, point string, data any) (any, error)

	// EmitWithStrategy 触发钩子并指定错误策略
	EmitWithStrategy(ctx context.Context, point string, data any, strategy ErrorStrategy) (any, []error)

	// LoadConfig 从配置加载 Hook
	LoadConfig(config structs.HookConfig) error

	// List 列出所有已注册的钩子点
	List() []string

	// Count 返回钩子点的钩子数量
	Count(point string) int

	// SetDebugEnabled 设置调试模式
	SetDebugEnabled(enabled bool)

	// ProcessManager 返回外部 Hook 进程管理器
	ProcessManager() *ProcessManager
}

// hookEngine Hook 引擎实现
type hookEngine struct {
	mu sync.RWMutex

	// 内部 Hook 注册表，实现 interfaces.Registry[*internalHookEntry]
	internalRegistry *InternalHookRegistry

	// 外部 Hook 进程管理器，实现 interfaces.Manager[*ExternalHook]
	processMgr *ProcessManager

	// 执行指标
	metrics map[string]*HookMetrics

	// 调试模式
	debug bool
}

// NewEngine 创建新的 Hook 引擎
func NewEngine() HookEngine {
	defaults := structs.DefaultHookConfig().Defaults
	return &hookEngine{
		internalRegistry: NewInternalHookRegistry(),
		processMgr:       NewProcessManager(defaults),
		metrics:          make(map[string]*HookMetrics),
		debug:            true,
	}
}

// GetEngine 获取全局 Hook 引擎（兼容旧 API）
func GetEngine() HookEngine {
	return globalHookEngine
}

// 全局 Hook 引擎实例
var globalHookEngine = NewEngine()

// ProcessManager 返回外部 Hook 进程管理器
func (e *hookEngine) ProcessManager() *ProcessManager {
	return e.processMgr
}

// Register 注册内部 Hook（兼容旧 API）
func (e *hookEngine) Register(point string, fn HookFunc) {
	e.RegisterWithPriority(point, PriorityNormal, fn)
}

// RegisterWithPriority 注册带优先级的内部 Hook（兼容旧 API）
func (e *hookEngine) RegisterWithPriority(point string, priority Priority, fn HookFunc) {
	e.mu.Lock()
	defer e.mu.Unlock()

	// 确保指标存在
	if _, exists := e.metrics[point]; !exists {
		e.metrics[point] = &HookMetrics{
			Point: point,
		}
	}

	// 获取或创建该点的条目
	entry := e.internalRegistry.GetOrCreate(point)

	// 创建钩子定义
	hookDef := HookDefinition{
		Func:     fn,
		Priority: priority,
		Index:    len(entry.Definitions),
	}

	// 按优先级插入到正确位置
	insertIndex := 0
	for i, existing := range entry.Definitions {
		if priority < existing.Priority {
			insertIndex = i
			break
		}
		insertIndex = i + 1
	}

	entry.Definitions = append(
		entry.Definitions[:insertIndex],
		append([]HookDefinition{hookDef}, entry.Definitions[insertIndex:]...)...,
	)

	// 更新索引
	for i := insertIndex + 1; i < len(entry.Definitions); i++ {
		entry.Definitions[i].Index = i
	}

	if e.debug {
		logger.L().Debug().
			Str("hook_point", point).
			Int("priority", int(priority)).
			Msg("Internal hook registered")
	}
}

// RegisterExternal 注册外部脚本 Hook
func (e *hookEngine) RegisterExternal(point string, config structs.ExternalConfig) error {
	// 验证配置
	if err := ValidateExternalHook(&config); err != nil {
		return err
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	// 确保指标存在
	if _, exists := e.metrics[point]; !exists {
		e.metrics[point] = &HookMetrics{
			Point: point,
		}
	}

	// 通过 ProcessManager 添加
	def := structs.HookDefinition{
		Point:    point,
		Type:     structs.HookTypeExternal,
		Enabled:  true,
		Priority: int(PriorityNormal),
		Config:   config,
	}

	if err := e.processMgr.AddFromConfig(def); err != nil {
		return err
	}

	if e.debug {
		logger.L().Debug().
			Str("hook_point", point).
			Str("script", config.ScriptPath).
			Msg("External hook registered")
	}

	return nil
}

// Unregister 注销内部 Hook（兼容旧 API）
func (e *hookEngine) Unregister(point string, fn HookFunc) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	defs := e.internalRegistry.GetDefinitions(point)
	if len(defs) == 0 {
		return fmt.Errorf("hook not found for point %s", point)
	}

	for i, hookDef := range defs {
		// 通过反射比较函数指针
		if reflect.ValueOf(hookDef.Func).Pointer() == reflect.ValueOf(fn).Pointer() {
			entry := e.internalRegistry.GetOrCreate(point)
			entry.Definitions = append(entry.Definitions[:i], entry.Definitions[i+1:]...)

			// 更新索引
			for j := i; j < len(entry.Definitions); j++ {
				entry.Definitions[j].Index = j
			}

			if e.debug {
				logger.L().Debug().
					Str("hook_point", point).
					Int("index", i).
					Msg("Internal hook unregistered")
			}

			return nil
		}
	}

	return fmt.Errorf("hook not found for point %s", point)
}

// UnregisterExternal 注销外部 Hook
func (e *hookEngine) UnregisterExternal(point string, scriptPath string) error {
	return e.processMgr.RemoveByPoint(point, scriptPath)
}

// Emit 触发钩子（流水线模式）
func (e *hookEngine) Emit(ctx context.Context, point string, data any) (any, error) {
	result, errs := e.EmitWithStrategy(ctx, point, data, ErrorStrategyStop)
	if len(errs) > 0 {
		return nil, errs[0]
	}
	return result, nil
}

// EmitWithStrategy 触发钩子并指定错误策略
func (e *hookEngine) EmitWithStrategy(ctx context.Context, point string, data any, strategy ErrorStrategy) (any, []error) {
	// 通过 Registry 获取内部 Hook
	internalDefs := e.internalRegistry.GetDefinitions(point)
	// 通过 ProcessManager 获取外部 Hook
	externalHooks := e.processMgr.GetByPoint(point)

	if len(internalDefs) == 0 && len(externalHooks) == 0 {
		if e.debug {
			logger.L().Debug().
				Str("hook_point", point).
				Msg("No hooks registered for point")
		}
		return data, nil
	}

	var errors []error
	currentData := data

	// 先执行内部 Hook
	for _, hookDef := range internalDefs {
		result, err := e.executeInternalHook(ctx, point, hookDef, currentData)
		if err != nil {
			hookErr := &HookError{
				Point:    point,
				Index:    hookDef.Index,
				Priority: hookDef.Priority,
				Input:    currentData,
				Err:      err,
			}

			switch strategy {
			case ErrorStrategyStop:
				return nil, append(errors, hookErr)
			case ErrorStrategyContinue:
				errors = append(errors, hookErr)
				continue
			case ErrorStrategyIgnore:
				continue
			}
		}

		currentData = result
	}

	// 再执行外部 Hook（通过 ProcessManager）
	for _, hook := range externalHooks {
		if !hook.Enabled() {
			continue
		}

		result, err := e.executeExternalHook(ctx, hook, currentData)
		if err != nil {
			hookErr := fmt.Errorf("external hook [%s] failed: %w", hook.Config().ScriptPath, err)

			switch strategy {
			case ErrorStrategyStop:
				return nil, append(errors, hookErr)
			case ErrorStrategyContinue:
				errors = append(errors, hookErr)
				continue
			case ErrorStrategyIgnore:
				continue
			}
		}

		currentData = result
	}

	if len(errors) > 0 {
		return currentData, errors
	}

	return currentData, nil
}

// executeInternalHook 执行内部 Hook
func (e *hookEngine) executeInternalHook(ctx context.Context, point string, hookDef HookDefinition, data any) (any, error) {
	startTime := time.Now()

	if e.debug {
		logger.L().Debug().
			Str("hook_point", point).
			Int("hook_index", hookDef.Index).
			Int("priority", int(hookDef.Priority)).
			Msg("Executing internal hook")
	}

	// 执行钩子
	result, err := hookDef.Func(ctx, data)

	// 更新指标
	duration := time.Since(startTime)
	e.mu.Lock()
	metrics := e.metrics[point]
	metrics.ExecutionCount++
	metrics.TotalDuration += duration
	metrics.LastExecution = startTime
	if err != nil {
		metrics.ErrorCount++
	}
	e.mu.Unlock()

	return result, err
}

// executeExternalHook 执行外部 Hook
func (e *hookEngine) executeExternalHook(ctx context.Context, hook *ExternalHook, data any) (any, error) {
	execResult, err := e.processMgr.executor.Execute(ctx, hook, data)
	if err != nil {
		return nil, err
	}

	if !execResult.success {
		return nil, execResult.err
	}

	response := execResult.response
	switch response.Decision {
	case DecisionAllow:
		return data, nil

	case DecisionDeny:
		return nil, fmt.Errorf("hook denied: %s", response.Message)

	case DecisionModify:
		if len(response.ModifiedData) > 0 {
			var modifiedData map[string]any
			if err := json.Unmarshal(response.ModifiedData, &modifiedData); err != nil {
				return nil, fmt.Errorf("failed to unmarshal modified data: %w", err)
			}
			return modifiedData, nil
		}
		return data, nil

	default:
		return nil, fmt.Errorf("unknown decision type: %s", response.Decision)
	}
}

// LoadConfig 从配置加载 Hook
func (e *hookEngine) LoadConfig(config structs.HookConfig) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	// 通过 ProcessManager 初始化（它内部会合并 defaults 并加载外部 Hook）
	ctx := context.Background()
	if err := e.processMgr.Initialize(ctx, config); err != nil {
		return fmt.Errorf("failed to initialize ProcessManager: %w", err)
	}

	return nil
}

// List 列出所有已注册的钩子点
func (e *hookEngine) List() []string {
	pointSet := make(map[string]bool)
	for _, p := range e.internalRegistry.Points() {
		pointSet[p] = true
	}
	for _, p := range e.processMgr.Points() {
		pointSet[p] = true
	}

	result := make([]string, 0, len(pointSet))
	for point := range pointSet {
		result = append(result, point)
	}
	return result
}

// Count 返回钩子点的钩子数量
func (e *hookEngine) Count(point string) int {
	count := len(e.internalRegistry.GetDefinitions(point))
	count += len(e.processMgr.GetByPoint(point))
	return count
}

// SetDebugEnabled 设置调试模式
func (e *hookEngine) SetDebugEnabled(enabled bool) {
	e.debug = enabled
	e.processMgr.SetDebugEnabled(enabled)
}
