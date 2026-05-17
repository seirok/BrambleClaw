package hook

import (
	"context"
	"fmt"
	"neoclaw/internal/config/structs"
	"neoclaw/internal/interfaces"
	"neoclaw/internal/logger"
	"sync"
)

// ProcessManager 管理外部 Hook 子进程的生命周期
// 实现 interfaces.Manager[*ExternalHook] 接口
type ProcessManager struct {
	mu       sync.RWMutex
	registry *ExternalHookRegistry
	executor *ExternalHookExecutor
	status   interfaces.ManagerStatus
	defaults structs.HookDefaults
}

// 编译时检查接口实现
var _ interfaces.Manager[*ExternalHook] = (*ProcessManager)(nil)

// NewProcessManager 创建新的 ProcessManager
func NewProcessManager(defaults structs.HookDefaults) *ProcessManager {
	return &ProcessManager{
		registry: NewExternalHookRegistry(),
		executor: NewExternalHookExecutor(),
		status:   interfaces.StatusIdle,
		defaults: defaults,
	}
}

// Initialize 从配置初始化外部 Hook
func (pm *ProcessManager) Initialize(ctx context.Context, cfg any) error {
	hookCfg, ok := cfg.(structs.HookConfig)
	if !ok {
		return fmt.Errorf("invalid config type for ProcessManager: expected HookConfig")
	}

	pm.mu.Lock()
	defer pm.mu.Unlock()

	// 更新默认配置
	pm.mergeDefaults(hookCfg.Defaults)

	// 加载外部 Hook 定义
	for _, def := range hookCfg.Definitions {
		if !def.Enabled || def.Type != structs.HookTypeExternal {
			continue
		}

		if err := pm.validateAndAdd(def); err != nil {
			logger.L().Error().
				Str("hook_point", def.Point).
				Str("script", def.Config.ScriptPath).
				Err(err).
				Msg("Failed to load external hook from config")
			continue
		}
	}

	pm.status = interfaces.StatusRunning

	logger.L().Info().
		Int("hook_count", len(pm.registry.List(ctx))).
		Msg("ProcessManager initialized")

	return nil
}

// StartAll 启动所有外部 Hook（目前为标记状态，进程按需创建）
func (pm *ProcessManager) StartAll(ctx context.Context) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if pm.status == interfaces.StatusRunning {
		return nil
	}

	pm.status = interfaces.StatusRunning

	logger.L().Info().Msg("ProcessManager started")
	return nil
}

// StopAll 停止所有外部 Hook 并清理资源
func (pm *ProcessManager) StopAll(ctx context.Context) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if pm.status == interfaces.StatusStopped {
		return nil
	}

	pm.status = interfaces.StatusStopped

	logger.L().Info().Msg("ProcessManager stopped")
	return nil
}

// Add 添加一个外部 Hook
func (pm *ProcessManager) Add(ctx context.Context, id string, item *ExternalHook) error {
	if item == nil {
		return fmt.Errorf("external hook cannot be nil")
	}

	pm.mu.Lock()
	defer pm.mu.Unlock()

	return pm.registry.Register(ctx, id, item)
}

// Remove 移除一个外部 Hook
func (pm *ProcessManager) Remove(ctx context.Context, id string) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	return pm.registry.Unregister(ctx, id)
}

// Get 获取一个外部 Hook
func (pm *ProcessManager) Get(ctx context.Context, id string) (*ExternalHook, error) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	return pm.registry.Get(ctx, id)
}

// List 列出所有外部 Hook
func (pm *ProcessManager) List(ctx context.Context) []*ExternalHook {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	return pm.registry.List(ctx)
}

// Status 返回管理器状态
func (pm *ProcessManager) Status() interfaces.ManagerStatus {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	return pm.status
}

// --- 扩展方法 ---

// Execute 执行指定钩子点的外部 Hook
func (pm *ProcessManager) Execute(ctx context.Context, point string, data any) (any, error) {
	pm.mu.RLock()
	if pm.status != interfaces.StatusRunning {
		pm.mu.RUnlock()
		return nil, fmt.Errorf("ProcessManager is not running (status: %d)", pm.status)
	}
	hooks := pm.registry.GetByPoint(point)
	pm.mu.RUnlock()

	if len(hooks) == 0 {
		return data, nil
	}

	currentData := data
	for _, h := range hooks {
		if !h.Enabled() {
			continue
		}

		result, err := pm.executor.Execute(ctx, h, currentData)
		if err != nil {
			return nil, err
		}

		if !result.success {
			return nil, result.err
		}

		// 根据决策处理
		switch result.response.Decision {
		case DecisionAllow:
			// 继续传递原始数据
		case DecisionDeny:
			return nil, fmt.Errorf("hook denied: %s", result.response.Message)
		case DecisionModify:
			if len(result.response.ModifiedData) > 0 {
				currentData = result.response.ModifiedData
			}
		}
	}

	return currentData, nil
}

// AddFromConfig 从配置添加外部 Hook
func (pm *ProcessManager) AddFromConfig(def structs.HookDefinition) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	return pm.validateAndAdd(def)
}

// RemoveByPoint 按钩子点和脚本路径移除外部 Hook
func (pm *ProcessManager) RemoveByPoint(point, scriptPath string) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	return pm.registry.RemoveHook(point, scriptPath)
}

// GetByPoint 获取指定钩子点的所有外部 Hook
func (pm *ProcessManager) GetByPoint(point string) []*ExternalHook {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	return pm.registry.GetByPoint(point)
}

// Points 列出所有有外部 Hook 的钩子点名称
func (pm *ProcessManager) Points() []string {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	return pm.registry.Points()
}

// SetDebugEnabled 设置调试模式
func (pm *ProcessManager) SetDebugEnabled(enabled bool) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	pm.executor.SetDebugEnabled(enabled)
}

// --- 内部方法 ---

// mergeDefaults 合并默认配置
func (pm *ProcessManager) mergeDefaults(defaults structs.HookDefaults) {
	if defaults.TimeoutMs > 0 {
		pm.defaults.TimeoutMs = defaults.TimeoutMs
	}
	if defaults.MaxOutputSize > 0 {
		pm.defaults.MaxOutputSize = defaults.MaxOutputSize
	}
	if defaults.Shell != "" {
		pm.defaults.Shell = defaults.Shell
	}
	if defaults.WorkingDir != "" {
		pm.defaults.WorkingDir = defaults.WorkingDir
	}
	if len(defaults.Env) > 0 {
		pm.defaults.Env = defaults.Env
	}
}

// validateAndAdd 验证并添加外部 Hook
func (pm *ProcessManager) validateAndAdd(def structs.HookDefinition) error {
	if err := ValidateExternalHook(&def.Config); err != nil {
		return err
	}

	hook := NewExternalHook(def.Point, Priority(def.Priority), def.Config, pm.defaults)
	pm.registry.AddHook(hook)

	return nil
}
